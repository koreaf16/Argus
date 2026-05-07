package workspace

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func localExecWorkingDir(raw string) (string, error) {
	wd := strings.TrimSpace(raw)
	if wd == "" {
		return os.Getwd()
	}
	if filepath.IsAbs(wd) {
		return filepath.Clean(wd), nil
	}
	abs, err := filepath.Abs(wd)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func (m *Manager) startLocalExecStreaming(ctx context.Context, command string, opts ExecOptions) (*ExecHandle, error) {
	var cmd *exec.Cmd

	shell := opts.Shell
	if shell == "" {
		if runtime.GOOS == "windows" {
			shell = "powershell"
		} else {
			shell = "bash"
		}
	}

	switch shell {
	case "powershell", "pwsh":
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", command)
	case "bash", "git-bash":
		cmd = exec.CommandContext(ctx, "bash", "-lc", command)
	default:
		// Unknown shell, fallback to OS default
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", command)
		} else {
			cmd = exec.CommandContext(ctx, "bash", "-lc", command)
		}
	}
	workDir, err := localExecWorkingDir(opts.WorkingDir)
	if err != nil {
		return nil, fmt.Errorf("resolve local working directory: %w", err)
	}
	cmd.Dir = workDir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("local exec stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("local exec stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
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
			CWD:    workDir,
		}
		close(result)
	}()

	return &ExecHandle{
		Stream: stream,
		Result: result,
		Write: func(s string) error {
			_, err := io.WriteString(stdin, s)
			return err
		},
		Kill: func() { _ = cmd.Process.Kill() },
	}, nil
}

func (m *Manager) execLocal(ctx context.Context, command string, opts ExecOptions) (ExecResult, error) {
	var cmd *exec.Cmd

	shell := opts.Shell
	if shell == "" {
		if runtime.GOOS == "windows" {
			shell = "powershell"
		} else {
			shell = "bash"
		}
	}

	switch shell {
	case "powershell", "pwsh":
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", command)
	case "bash", "git-bash":
		cmd = exec.CommandContext(ctx, "bash", "-lc", command)
	default:
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", command)
		} else {
			cmd = exec.CommandContext(ctx, "bash", "-lc", command)
		}
	}
	workDir, err := localExecWorkingDir(opts.WorkingDir)
	if err != nil {
		return ExecResult{}, fmt.Errorf("resolve local working directory: %w", err)
	}
	cmd.Dir = workDir

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
		CWD:    workDir,
	}
	if err != nil && code == 0 {
		return res, fmt.Errorf("execute local command: %w", err)
	}
	return res, nil
}
