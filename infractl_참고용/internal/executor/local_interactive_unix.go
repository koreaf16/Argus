//go:build !windows

package executor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/creack/pty"
)

// localInteractiveSession implements ExecSession for interactive local commands.
type localInteractiveSession struct {
	ctx        context.Context
	cancel     context.CancelFunc
	cmd        *exec.Cmd
	ptmx       *os.File
	done       chan struct{}
	result     ExecResult
	err        error
	start      time.Time
	cleanupFns []func() error
	onChunk    func(string)
}

func (s *localInteractiveSession) InjectStdin(line string) error {
	_, err := fmt.Fprintln(s.ptmx, line)
	return err
}

func (s *localInteractiveSession) SendEOF() error {
	_, err := s.ptmx.Write([]byte{0x04})
	return err
}

func (s *localInteractiveSession) Wait() (ExecResult, error) {
	<-s.done
	return s.result, s.err
}

func (s *localInteractiveSession) run() {
	defer close(s.done)
	defer s.cancel()
	defer s.ptmx.Close()
	defer runCleanups(s.cleanupFns)

	var output []byte
	buf := make([]byte, 1024)
	for {
		n, readErr := s.ptmx.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			output = append(output, chunk...)
			if s.onChunk != nil {
				s.onChunk(string(chunk))
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				s.err = fmt.Errorf("read PTY output: %w", readErr)
			}
			break
		}
	}

	waitErr := s.cmd.Wait()
	duration := time.Since(s.start)

	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			s.err = fmt.Errorf("wait PTY command: %w", waitErr)
			return
		}
	}

	s.result = ExecResult{
		Stdout:   TruncateOutput(string(output), MaxOutputBytes),
		ExitCode: exitCode,
		Duration: duration,
	}
}

// ExecuteInteractive runs a local command in a PTY and returns a session for stdin control.
func (e *LocalExecutor) ExecuteInteractive(ctx context.Context, spec InteractiveSpec, onChunk func(string)) (ExecSession, error) {
	var cancel context.CancelFunc
	if _, ok := ctx.Deadline(); ok {
		ctx, cancel = context.WithCancel(ctx)
	} else {
		ctx, cancel = context.WithTimeout(ctx, e.timeout)
	}

	prepared, err := buildCommand(ctx, spec.Command)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("build command: %w", err)
	}
	cmd := exec.CommandContext(ctx, prepared.Argv[0], prepared.Argv[1:]...)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		cancel()
		runCleanups(prepared.CleanupFns)
		return nil, fmt.Errorf("start PTY command: %w", err)
	}

	session := &localInteractiveSession{
		ctx:        ctx,
		cancel:     cancel,
		cmd:        cmd,
		ptmx:       ptmx,
		done:       make(chan struct{}),
		start:      time.Now(),
		cleanupFns: prepared.CleanupFns,
		onChunk:    onChunk,
	}
	go session.run()
	return session, nil
}
