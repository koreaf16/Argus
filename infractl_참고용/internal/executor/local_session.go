// Package executor
// File: local_session.go
// Description: 로컬 명령어 스트리밍 세션
// Responsibility: localSession 구조체와 ExecuteStream을 통한 비동기 스트리밍 실행

package executor

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// localSession implements ExecSession for local commands.
type localSession struct {
	ctx          context.Context
	cancel       context.CancelFunc
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	mode         stdinMode
	done         chan struct{}
	result       ExecResult
	err          error
	start        time.Time
	stdoutCh     <-chan string
	stderrCh     <-chan string
	onLine       func(string)
	cleanupStdin func()
	cleanupFns   []func() error // PreparedCmd.CleanupFns (스냅샷 파일 삭제 등)
}

func (s *localSession) InjectStdin(line string) error {
	if s.stdin == nil {
		return fmt.Errorf("inject stdin: no stdin pipe")
	}
	var writeErr error
	if s.mode == stdinModePTY && runtime.GOOS == "windows" {
		_, writeErr = fmt.Fprintf(s.stdin, "%s\r\n", line)
	} else {
		_, writeErr = fmt.Fprintln(s.stdin, line)
	}
	return writeErr
}

func (s *localSession) SendEOF() error {
	if s.stdin == nil {
		return fmt.Errorf("send EOF: no stdin pipe")
	}
	if s.mode == stdinModePTY {
		_, err := s.stdin.Write([]byte{0x04})
		return err
	}
	return s.stdin.Close()
}

func (s *localSession) Wait() (ExecResult, error) {
	<-s.done
	return s.result, s.err
}

func (s *localSession) run() {
	defer close(s.done)
	defer s.cancel()
	if s.cleanupStdin != nil {
		defer s.cleanupStdin()
	}
	defer runCleanups(s.cleanupFns)

	var stdoutLines, stderrLines []string
	var stdoutSize, stderrSize int
	stdoutDone := make(chan struct{})
	stderrDone := make(chan struct{})

	go func() {
		for line := range s.stdoutCh {
			if stdoutSize < MaxOutputBytes {
				stdoutLines = append(stdoutLines, line)
				stdoutSize += len(line) + 1 // +1 for newline
			}
			if s.onLine != nil {
				s.onLine(line)
			}
		}
		close(stdoutDone)
	}()

	go func() {
		for line := range s.stderrCh {
			if stderrSize < MaxOutputBytes {
				stderrLines = append(stderrLines, line)
				stderrSize += len(line) + 1
			}
			if s.onLine != nil {
				s.onLine(line)
			}
		}
		close(stderrDone)
	}()

	s.err = s.cmd.Start()
	if s.err != nil {
		return
	}

	<-stdoutDone
	<-stderrDone

	waitErr := s.cmd.Wait()
	duration := time.Since(s.start)

	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if context.Cause(s.ctx) != nil {
			// Command was killed because its context was cancelled/timed out.
			// Treat this as a non-zero exit rather than an error so callers see
			// whatever partial output was collected rather than a raw "context
			// canceled" string.
			exitCode = 1
		} else {
			s.err = fmt.Errorf("wait command: %w", waitErr)
			return
		}
	}

	s.result = ExecResult{
		Stdout:   TruncateOutput(strings.Join(stdoutLines, "\n"), MaxOutputBytes),
		Stderr:   TruncateOutput(strings.Join(stderrLines, "\n"), MaxOutputBytes),
		ExitCode: exitCode,
		Duration: duration,
	}
}

// ExecuteStream runs a local command, streaming stdout and stderr line-by-line via onLine.
// Returns an ExecSession whose Wait() blocks until the command finishes.
// InjectStdin and SendEOF may be called concurrently to interact with the running command.
func (e *LocalExecutor) ExecuteStream(ctx context.Context, command string, onLine func(string)) (ExecSession, error) {
	var cancel context.CancelFunc
	if _, ok := ctx.Deadline(); ok {
		ctx, cancel = context.WithCancel(ctx)
	} else {
		ctx, cancel = context.WithTimeout(ctx, e.timeout)
	}

	prepared, err := buildCommand(ctx, command)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("build command: %w", err)
	}
	cmd := exec.CommandContext(ctx, prepared.Argv[0], prepared.Argv[1:]...)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}

	e.mu.Lock()
	e.activeStdin = stdinPipe
	e.activeStdinMode = stdinModePipe
	e.mu.Unlock()

	session := &localSession{
		ctx:      ctx,
		cancel:   cancel,
		cmd:      cmd,
		stdin:    stdinPipe,
		mode:     stdinModePipe,
		done:     make(chan struct{}),
		start:    time.Now(),
		stdoutCh: StartLineAssembler(StartPipeReader(stdoutPipe)),
		stderrCh: StartLineAssembler(StartPipeReader(stderrPipe)),
		onLine:   onLine,
		cleanupFns: prepared.CleanupFns,
		cleanupStdin: func() {
			e.mu.Lock()
			if e.activeStdin == stdinPipe {
				e.activeStdin = nil
			}
			e.mu.Unlock()
		},
	}
	go session.run()
	return session, nil
}
