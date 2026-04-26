//go:build windows

// Package executor
// File: local_pty_windows.go
// Description: ConPTY-backed stream execution for local commands on Windows
// Responsibility: Allocate a Windows ConPTY so interactive prompts (scp, ssh) appear in stdout

package executor

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/creack/pty"
	"github.com/yourorg/infractl/internal/executor/shell/powershell"
)

// ExecuteStreamPTY runs a local command in a ConPTY while streaming lines.
// ConPTY ensures password prompts from scp/ssh reach stdout,
// enabling the idle handler to detect and auto-respond.
// Falls back to regular ExecuteStream if ConPTY is unavailable.
func (e *LocalExecutor) ExecuteStreamPTY(ctx context.Context, command string, onLine func(string)) (ExecSession, error) {
	outerCtx := ctx // preserve for fallback before deriving a cancellable child
	var cancel context.CancelFunc
	if _, ok := ctx.Deadline(); ok {
		ctx, cancel = context.WithCancel(ctx)
	} else {
		ctx, cancel = context.WithTimeout(ctx, e.timeout)
	}

	// ConPTY에서는 -NonInteractive를 제거하여 자식 프로세스가 PTY로 상호작용 가능하게 한다.
	cmd := buildCommandPTY(ctx, command)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		cancel()
		slog.Warn("conpty start failed, falling back to pipe execution (password prompts may not appear)", "err", err)
		// Use the original (non-cancelled) context so the fallback command can run normally.
		return e.ExecuteStream(outerCtx, command, onLine)
	}

	e.setActivePTY(ptmx)

	s := &localPTYSession{
		ctx:      ctx,
		cancel:   cancel,
		cmd:      cmd,
		ptmx:     ptmx,
		done:     make(chan struct{}),
		start:    time.Now(),
		stdoutCh: StartLineAssembler(StartPipeReader(ptmx)),
		onLine:   onLine,
		cleanupStdin: func() {
			e.clearActiveStdin(ptmx)
		},
	}
	go s.run()
	return s, nil
}

// localPTYSession implements ExecSession for ConPTY-based commands.
type localPTYSession struct {
	ctx          context.Context
	cancel       context.CancelFunc
	cmd          *exec.Cmd
	ptmx         *os.File
	done         chan struct{}
	result       ExecResult
	err          error
	start        time.Time
	stdoutCh     <-chan string
	onLine       func(string)
	cleanupStdin func()
}

func (s *localPTYSession) InjectStdin(line string) error {
	if s.ptmx == nil {
		return nil
	}
	_, err := s.ptmx.Write([]byte(line + "\r\n"))
	return err
}

func (s *localPTYSession) SendEOF() error {
	if s.ptmx == nil {
		return nil
	}
	_, err := s.ptmx.Write([]byte{0x04})
	return err
}

func (s *localPTYSession) Wait() (ExecResult, error) {
	<-s.done
	return s.result, s.err
}

func (s *localPTYSession) run() {
	defer close(s.done)
	defer s.cancel()
	if s.cleanupStdin != nil {
		defer s.cleanupStdin()
	}
	defer s.ptmx.Close()

	var lines []string
	for line := range s.stdoutCh {
		line = StripANSI(line)
		lines = append(lines, line)
		if s.onLine != nil {
			s.onLine(line)
		}
	}

	waitErr := s.cmd.Wait()
	duration := time.Since(s.start)

	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	s.result = ExecResult{
		Stdout:   TruncateOutput(strings.Join(lines, "\n"), MaxOutputBytes),
		ExitCode: exitCode,
		Duration: duration,
	}
}

// buildCommandPTY creates a command for ConPTY execution.
// -NonInteractive를 제거하여 자식 프로세스의 콘솔 입출력이 ConPTY를 통과하게 한다.
func buildCommandPTY(ctx context.Context, command string) *exec.Cmd {
	psCmd := "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; " +
		"$OutputEncoding = [System.Text.Encoding]::UTF8; " + command
	return exec.CommandContext(ctx,
		"powershell.exe",
		"-NoProfile",
		"-EncodedCommand", powershell.EncodeUTF16LE(psCmd),
	)
}
