// Package executor
// File: local.go
// Description: 로컬 명령어 실행기 — 구조체, 생성자, 동기 실행, stdin 제어
// Responsibility: LocalExecutor의 핵심 정의와 동기 명령어 실행

package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	_ "github.com/yourorg/infractl/internal/executor/shell/bash"       // auto-register bash provider
	_ "github.com/yourorg/infractl/internal/executor/shell/powershell" // auto-register powershell provider

	"github.com/yourorg/infractl/internal/executor/shell"
)

// stdinMode describes how the active stdin pipe should handle line endings for injection.
type stdinMode int

const (
	stdinModePipe stdinMode = iota // regular OS pipe — LF (\n) is the newline
	stdinModePTY                   // PTY / ConPTY — CR (\r) or CR+LF is required for Enter
)

const defaultTimeout = 30 * time.Second

// LocalExecutor runs commands on the local controller.
type LocalExecutor struct {
	timeout         time.Duration
	mu              sync.Mutex
	activeStdin     io.WriteCloser
	activeStdinMode stdinMode
}

// NewLocalExecutor builds a local executor with a default timeout.
func NewLocalExecutor(timeout time.Duration) *LocalExecutor {
	if timeout == 0 {
		timeout = defaultTimeout
	}
	return &LocalExecutor{timeout: timeout}
}

// Execute runs a command and buffers stdout/stderr until completion.
func (e *LocalExecutor) Execute(ctx context.Context, command string) (ExecResult, error) {
	start := time.Now()

	var cancel context.CancelFunc
	if _, ok := ctx.Deadline(); ok {
		ctx, cancel = context.WithCancel(ctx)
	} else {
		ctx, cancel = context.WithTimeout(ctx, e.timeout)
	}
	defer cancel()

	prepared, err := buildCommand(ctx, command)
	if err != nil {
		return ExecResult{}, fmt.Errorf("build command: %w", err)
	}
	defer runCleanups(prepared.CleanupFns)

	cmd := exec.CommandContext(ctx, prepared.Argv[0], prepared.Argv[1:]...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &LimitedWriter{Buf: &stdoutBuf, Limit: MaxOutputBytes}
	cmd.Stderr = &LimitedWriter{Buf: &stderrBuf, Limit: MaxOutputBytes}

	runErr := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return ExecResult{}, fmt.Errorf("execute command: %w", runErr)
		}
	}

	return ExecResult{
		Stdout:   TruncateOutput(stdoutBuf.String(), MaxOutputBytes),
		Stderr:   TruncateOutput(stderrBuf.String(), MaxOutputBytes),
		ExitCode: exitCode,
		Duration: duration,
	}, nil
}

// Target returns the local execution label.
func (e *LocalExecutor) Target() string {
	return "localhost"
}

// Host returns "localhost" for the local executor.
func (e *LocalExecutor) Host() string {
	return "localhost"
}

func (e *LocalExecutor) WorkspaceDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

// Platform returns the local controller platform.
func (e *LocalExecutor) Platform() Platform {
	return NormalizePlatform(runtime.GOOS)
}

// ShellName returns the local shell label.
func (e *LocalExecutor) ShellName() string {
	return LocalShellName()
}

// InjectStdin sends a line to the currently running command's stdin.
func (e *LocalExecutor) InjectStdin(line string) error {
	e.mu.Lock()
	stdin := e.activeStdin
	mode := e.activeStdinMode
	e.mu.Unlock()

	if stdin == nil {
		return fmt.Errorf("inject stdin: no active command")
	}
	if mode == stdinModePTY && runtime.GOOS == "windows" {
		_, err := fmt.Fprintf(stdin, "%s\r\n", line)
		return err
	}
	_, err := fmt.Fprintln(stdin, line)
	return err
}

// SendEOF signals EOF on the currently running command's stdin.
func (e *LocalExecutor) SendEOF() error {
	e.mu.Lock()
	stdin := e.activeStdin
	mode := e.activeStdinMode
	e.mu.Unlock()

	if stdin == nil {
		return fmt.Errorf("send EOF: no active command")
	}
	if mode == stdinModePTY {
		_, err := stdin.Write([]byte{0x04})
		return err
	}
	err := stdin.Close()
	if err == nil {
		e.mu.Lock()
		if e.activeStdin == stdin {
			e.activeStdin = nil
		}
		e.mu.Unlock()
	}
	return err
}

// setActivePTY registers a PTY file descriptor as the active stdin for the current command.
func (e *LocalExecutor) setActivePTY(ptmx io.WriteCloser) {
	e.mu.Lock()
	e.activeStdin = ptmx
	e.activeStdinMode = stdinModePTY
	e.mu.Unlock()
}

// clearActiveStdin clears the active stdin if it matches the provided writer.
func (e *LocalExecutor) clearActiveStdin(w io.WriteCloser) {
	e.mu.Lock()
	if e.activeStdin == w {
		e.activeStdin = nil
	}
	e.mu.Unlock()
}

// buildCommand uses the registered ShellProvider to prepare a command for execution.
// On Windows the powershell provider is used; on other platforms, bash.
// Ported from: claude_cli/src/utils/shell/bashProvider.ts:buildExecCommand
func buildCommand(ctx context.Context, command string) (shell.PreparedCmd, error) {
	p, err := shell.Resolve()
	if err != nil {
		return shell.PreparedCmd{}, fmt.Errorf("resolve shell provider: %w", err)
	}
	return p.Prepare(ctx, command)
}

// runCleanups executes all cleanup functions, logging but not propagating errors.
func runCleanups(fns []func() error) {
	for _, fn := range fns {
		if err := fn(); err != nil {
			slog.Warn("cleanup error", "err", err)
		}
	}
}
