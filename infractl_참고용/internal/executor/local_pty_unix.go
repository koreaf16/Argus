//go:build !windows

// Package executor
// File: local_pty_unix.go
// Description: PTY-backed stream execution for local commands on Unix
// Responsibility: Allocate a Unix PTY so interactive prompts (sudo, su, scp) appear in stdout

package executor

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/creack/pty"
)

// ExecuteStreamPTY runs a local command in a PTY while streaming lines.
func (e *LocalExecutor) ExecuteStreamPTY(ctx context.Context, command string, onLine func(string)) (ExecSession, error) {
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
	ptmx, err := pty.Start(cmd)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("start local pty: %w", err)
	}

	session := &localSession{
		ctx:        ctx,
		cancel:     cancel,
		cmd:        cmd,
		stdin:      ptmx,
		mode:       stdinModePTY,
		done:       make(chan struct{}),
		start:      time.Now(),
		cleanupFns: prepared.CleanupFns,
		stdoutCh:   StartLineAssembler(StartPipeReader(ptmx)),
		onLine: func(line string) {
			line = StripANSI(line)
			if onLine != nil {
				onLine(line)
			}
		},
	}
	go session.run()
	return session, nil
}
