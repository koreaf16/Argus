//go:build windows

package utils

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// RunCommand 는 Windows 에서 프로세스를 실행한다.
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

	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()
	stdinPipe, _ := cmd.StdinPipe()

	resultCh := make(chan ExecResult, 1)
	streamCh := make(chan string, 100)

	var once sync.Once
	killFn := func() {
		once.Do(func() {
			if cmd.Process != nil {
				// ISSUE-05 해결: Windows 에서 자식 프로세스 트리 전체 종료
				_ = exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
			}
			cancelTimeout()
		})
	}

	sc := &ShellCommand{
		Result:     resultCh,
		Stream:     streamCh,
		Stdin:      stdinPipe,
		Kill:       killFn,
		Status:     ShellStatusRunning,
		Cleanup:    func() {},
		Background: func(string) bool { return false },
		isPTY:      usePTY,
	}

	go func() {
		defer cancelTimeout()
		defer close(streamCh)

		if err := cmd.Start(); err != nil {
			sc.Status = ShellStatusKilled
			resultCh <- ExecResult{Code: 1, PreSpawnError: err.Error()}
			return
		}

		var stdoutFull, stderrFull bytes.Buffer
		var wg sync.WaitGroup
		wg.Add(2)

		readStream := func(r io.Reader, buf *bytes.Buffer) {
			defer wg.Done()
			reader := bufio.NewReader(r)
			for {
				chunk := make([]byte, 1024)
				n, err := reader.Read(chunk)
				if n > 0 {
					data := string(chunk[:n])
					buf.Write(chunk[:n])
					streamCh <- data
				}
				if err != nil {
					break
				}
			}
		}

		go readStream(stdoutPipe, &stdoutFull)
		go readStream(stderrPipe, &stderrFull)

		err := cmd.Wait()
		wg.Wait()

		code := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			} else {
				code = 1
			}
		}

		sc.Status = ShellStatusCompleted
		resultCh <- ExecResult{
			Stdout:      stdoutFull.String(),
			Stderr:      stderrFull.String(),
			Code:        code,
			Interrupted: ctx.Err() != nil,
		}
	}()

	return sc
}
