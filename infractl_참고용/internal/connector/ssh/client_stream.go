// Package ssh
// File: client_stream.go
// Description: SSH 스트리밍/인터랙티브 명령 실행 메서드
// Responsibility: Client의 RunStream, RunInteractive 메서드 제공

package ssh

import (
	"context"
	"fmt"
	"time"

	"github.com/yourorg/infractl/internal/executor"
)

// RunStream starts a command on the remote host and returns an ExecSession.
func (c *Client) RunStream(
	ctx context.Context,
	command string,
	requirePTY bool,
	onLine func(string),
	onIdle func(executor.StdinInjector),
) (executor.ExecSession, error) {
	if err := c.ensureConnected(ctx); err != nil {
		return nil, err
	}

	timeout := c.cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	var cancel context.CancelFunc
	if _, ok := ctx.Deadline(); ok {
		ctx, cancel = context.WithCancel(ctx)
	} else {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	}

	session, err := c.newSession(ctx)
	if err != nil {
		cancel()
		return nil, err
	}

	if requirePTY {
		if err := requestPTY(session); err != nil {
			cancel()
			session.Close()
			return nil, err
		}
	}

	stdinPipe, err := session.StdinPipe()
	if err != nil {
		cancel()
		session.Close()
		return nil, fmt.Errorf("ssh stdin pipe: %w", err)
	}

	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		cancel()
		session.Close()
		return nil, fmt.Errorf("ssh stdout pipe: %w", err)
	}

	if err := session.Start(wrapLoginShell(command)); err != nil {
		cancel()
		session.Close()
		return nil, fmt.Errorf("ssh start command: %w", err)
	}

	var watcher *executor.IdleWatcher
	if onIdle != nil {
		watcher = executor.NewIdleWatcher(0)
		watcher.Start(ctx)
	}

	s := &sshSession{
		ctx:      ctx,
		cancel:   cancel,
		session:  session,
		stdin:    stdinPipe,
		done:     make(chan struct{}),
		start:    time.Now(),
		stdoutCh: executor.StartLineAssembler(executor.StartPipeReader(stdoutPipe)),
		onLine:   onLine,
		pty:      requirePTY,
		watcher:  watcher,
	}

	go s.closeSessionOnContextDone()

	if onIdle != nil {
		go func() {
			select {
			case <-watcher.Triggered():
				onIdle(s)
			case <-ctx.Done():
			}
		}()
	}

	go s.runStream()
	return s, nil
}

// RunInteractive starts an SSH command and streams raw terminal chunks.
func (c *Client) RunInteractive(
	ctx context.Context,
	command string,
	requirePTY bool,
	onChunk func(string),
) (executor.ExecSession, error) {
	if err := c.ensureConnected(ctx); err != nil {
		return nil, err
	}

	timeout := c.cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	var cancel context.CancelFunc
	if _, ok := ctx.Deadline(); ok {
		ctx, cancel = context.WithCancel(ctx)
	} else {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	}

	session, err := c.newSession(ctx)
	if err != nil {
		cancel()
		return nil, err
	}

	if requirePTY {
		if err := requestPTY(session); err != nil {
			cancel()
			session.Close()
			return nil, err
		}
	}

	stdinPipe, err := session.StdinPipe()
	if err != nil {
		cancel()
		session.Close()
		return nil, fmt.Errorf("ssh stdin pipe: %w", err)
	}

	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		cancel()
		session.Close()
		return nil, fmt.Errorf("ssh stdout pipe: %w", err)
	}

	if err := session.Start(wrapLoginShell(command)); err != nil {
		cancel()
		session.Close()
		return nil, fmt.Errorf("ssh start command: %w", err)
	}

	s := &sshSession{
		ctx:       ctx,
		cancel:    cancel,
		session:   session,
		stdin:     stdinPipe,
		stdoutRaw: stdoutPipe,
		done:      make(chan struct{}),
		start:     time.Now(),
		onChunk:   onChunk,
		pty:       requirePTY,
	}

	go s.closeSessionOnContextDone()

	go s.runInteractive()
	return s, nil
}
