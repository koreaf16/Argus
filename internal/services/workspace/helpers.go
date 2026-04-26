package workspace

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (m *Manager) startLocalExecStreaming(ctx context.Context, command string) (*ExecHandle, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", command)
	} else {
		cmd = exec.CommandContext(ctx, "bash", "-lc", command)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("local exec stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdout.Close()
		return nil, fmt.Errorf("local exec stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("local exec start: %w", err)
	}

	stream := make(chan string, 16)
	result := make(chan ExecResult, 1)

	go func() {
		defer close(stream)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text() + "\n"
			select {
			case stream <- line:
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		var errBuf strings.Builder
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text() + "\n"
			errBuf.WriteString(line)
			select {
			case stream <- line:
			case <-ctx.Done():
				return
			}
		}
		waitErr := cmd.Wait()
		code := 0
		if waitErr != nil {
			if exitErr, ok := waitErr.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			} else {
				code = 1
			}
		}
		result <- ExecResult{
			Stdout: "",
			Stderr: errBuf.String(),
			Code:   code,
		}
		close(result)
	}()

	return &ExecHandle{
		Stream: stream,
		Result: result,
		Write:  func(string) error { return fmt.Errorf("stdin not supported for local exec") },
		Kill:   func() { _ = cmd.Process.Kill() },
	}, nil
}

func (m *Manager) execLocal(ctx context.Context, command string) (ExecResult, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", command)
	} else {
		cmd = exec.CommandContext(ctx, "bash", "-lc", command)
	}
	output, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			code = 1
		}
	}
	res := ExecResult{
		Stdout: string(output),
		Code:   code,
	}
	if err != nil && code == 0 {
		return res, fmt.Errorf("execute local command: %w", err)
	}
	return res, nil
}
