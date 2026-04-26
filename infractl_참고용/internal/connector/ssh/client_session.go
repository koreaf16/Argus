// Package ssh
// File: client_session.go
// Description: SSH 세션 래퍼 — 스트리밍/인터랙티브 명령의 실행 및 결과 수집
// Responsibility: sshSession 구조체와 InjectStdin, SendEOF, Wait, runStream, runInteractive 메서드 제공

package ssh

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/executor"
	gossh "golang.org/x/crypto/ssh"
)

// sshSession implements executor.ExecSession for streaming SSH commands.
type sshSession struct {
	ctx       context.Context
	cancel    context.CancelFunc
	session   *gossh.Session
	stdin     io.WriteCloser
	stdoutRaw io.Reader // used only by runInteractive
	done      chan struct{}
	result    executor.ExecResult
	err       error
	start     time.Time
	stdoutCh  <-chan string // used only by runStream
	onLine    func(string)
	onChunk   func(string)
	pty       bool
	watcher   *executor.IdleWatcher
}

// InjectStdin sends a line to the running command's stdin.
func (s *sshSession) InjectStdin(line string) error {
	if s.stdin == nil {
		return fmt.Errorf("inject stdin: no stdin pipe")
	}
	_, err := fmt.Fprintln(s.stdin, line)
	return err
}

// SendEOF signals EOF on the running command's stdin.
func (s *sshSession) SendEOF() error {
	if s.stdin == nil {
		return fmt.Errorf("send EOF: no stdin pipe")
	}
	return s.stdin.Close()
}

// Wait blocks until the command finishes and returns the final result.
func (s *sshSession) Wait() (executor.ExecResult, error) {
	<-s.done
	return s.result, s.err
}

func (s *sshSession) closeSessionOnContextDone() {
	select {
	case <-s.ctx.Done():
		_ = s.session.Close()
	case <-s.done:
	}
}

// runStream reads stdout lines until the channel closes, calling onLine for each.
func (s *sshSession) runStream() {
	defer s.cancel()
	defer close(s.done)

	var lines []string
	for line := range s.stdoutCh {
		lines = append(lines, line)
		if s.onLine != nil {
			s.onLine(line)
		}
		if s.watcher != nil {
			s.watcher.Ping()
		}
	}

	waitErr := s.session.Wait()
	duration := time.Since(s.start)

	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*gossh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			s.err = fmt.Errorf("ssh session wait: %w", waitErr)
		}
	}

	stdout := executor.TruncateOutput(strings.Join(lines, "\n"), executor.MaxOutputBytes)
	s.result = executor.ExecResult{
		Stdout:   stdout,
		ExitCode: exitCode,
		Duration: duration,
	}
}

// runInteractive reads raw stdout bytes and sends chunks to onChunk.
func (s *sshSession) runInteractive() {
	defer s.cancel()
	defer close(s.done)

	var output bytes.Buffer
	buf := make([]byte, 4096)
	for {
		n, readErr := s.stdoutRaw.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			output.WriteString(chunk)
			if s.onChunk != nil {
				s.onChunk(chunk)
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				slog.Debug("ssh interactive read", "err", readErr)
			}
			break
		}
	}

	waitErr := s.session.Wait()
	duration := time.Since(s.start)

	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*gossh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			s.err = fmt.Errorf("ssh interactive wait: %w", waitErr)
		}
	}

	s.result = executor.ExecResult{
		Stdout:   executor.TruncateOutput(output.String(), executor.MaxOutputBytes),
		ExitCode: exitCode,
		Duration: duration,
	}
}
