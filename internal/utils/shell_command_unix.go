//go:build !windows

package utils

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// RunCommand 는 Unix 계열에서 PTY 를 사용하여 프로세스를 실행한다.
func RunCommand(
	ctx context.Context,
	name string,
	args []string,
	env map[string]string,
	cwd string,
	timeout time.Duration,
	usePTY bool,
) *ShellCommand {
	execCtx := ctx
	var cancelTimeout context.CancelFunc
	if timeout > 0 {
		execCtx, cancelTimeout = context.WithTimeout(ctx, timeout)
	} else {
		execCtx, cancelTimeout = context.WithCancel(ctx)
	}

	cmd := exec.CommandContext(execCtx, name, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}

	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// PTY 미사용 시 기존 방식
	if !usePTY {
		return runStandard(execCtx, cancelTimeout, cmd)
	}

	// PTY 사용 시
	f, err := pty.Start(cmd)
	if err != nil {
		return CreateFailedCommand(err)
	}

	resultCh := make(chan ExecResult, 1)
	streamCh := make(chan string, 100)

	var once sync.Once
	killFn := func() {
		once.Do(func() {
			if cmd.Process != nil {
				// ISSUE-05 해결: 프로세스 그룹 전체 종료
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
			_ = f.Close()
			cancelTimeout()
		})
	}

	sc := &ShellCommand{
		Result:     resultCh,
		Stream:     streamCh,
		Stdin:      f,
		Kill:       killFn,
		Status:     ShellStatusRunning,
		Cleanup:    func() { _ = f.Close() },
		Background: func(string) bool { return false },
		isPTY:      true,
	}

	go func() {
		defer cancelTimeout()
		defer close(streamCh)
		defer f.Close()

		var fullOutput bytes.Buffer
		reader := f
		for {
			buf := make([]byte, 1024)
			n, err := reader.Read(buf)
			if n > 0 {
				data := string(buf[:n])
				fullOutput.Write(buf[:n])
				streamCh <- data
			}
			if err != nil {
				break
			}
		}

		_ = cmd.Wait()
		code := cmd.ProcessState.ExitCode()

		sc.Status = ShellStatusCompleted
		resultCh <- ExecResult{
			Stdout:      fullOutput.String(),
			Code:        code,
			Interrupted: ctx.Err() != nil,
		}
	}()

	return sc
}

func runStandard(ctx context.Context, cancel context.CancelFunc, cmd *exec.Cmd) *ShellCommand {
	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()
	stdinPipe, _ := cmd.StdinPipe()

	resultCh := make(chan ExecResult, 1)
	streamCh := make(chan string, 100)

	killFn := func() {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		cancel()
	}

	sc := &ShellCommand{
		Result:  resultCh,
		Stream:  streamCh,
		Stdin:   stdinPipe,
		Kill:    killFn,
		Status:  ShellStatusRunning,
		Cleanup: func() {},
	}

	go func() {
		defer cancel()
		defer close(streamCh)
		if err := cmd.Start(); err != nil {
			resultCh <- ExecResult{Code: 1, PreSpawnError: err.Error()}
			return
		}
		var stdoutFull, stderrFull bytes.Buffer
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); io.Copy(io.MultiWriter(&stdoutFull, writerWrapper{streamCh}), stdoutPipe) }()
		go func() { defer wg.Done(); io.Copy(io.MultiWriter(&stderrFull, writerWrapper{streamCh}), stderrPipe) }()
		_ = cmd.Wait()
		wg.Wait()
		resultCh <- ExecResult{Stdout: stdoutFull.String(), Stderr: stderrFull.String(), Code: cmd.ProcessState.ExitCode()}
	}()
	return sc
}

type writerWrapper struct {
	ch chan string
}

func (w writerWrapper) Write(p []byte) (n int, err error) {
	w.ch <- string(p)
	return len(p), nil
}
