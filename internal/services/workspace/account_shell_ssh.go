package workspace

import (
	"context"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/ssh"
)

func (s *sshSession) openAccountShell(ctx context.Context, opts ExecOptions) (*accountShellSession, error) {
	session, err := s.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH account shell channel: %w", err)
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", 120, 40, modes); err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("failed to request account shell PTY: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("failed to open account shell stdin: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("failed to open account shell stdout: %w", err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("failed to open account shell stderr: %w", err)
	}

	targetUser := strings.TrimSpace(opts.AsUser)
	currentUser := strings.TrimSpace(s.currentUser())
	if targetUser == "" {
		targetUser = currentUser
	}
	method := normalizePrivilegeMethod(opts.PrivilegeMethod)
	startCommand := "bash -li"
	if targetUser != "" && !strings.EqualFold(targetUser, currentUser) {
		switch method {
		case PrivilegeSU:
			startCommand = "su -s /bin/bash - " + shellQuote(targetUser)
		default:
			method = PrivilegeSudo
			startCommand = "sudo -u " + shellQuote(targetUser) + " bash -li"
		}
	} else {
		method = PrivilegeNone
	}

	if err := session.Start(startCommand); err != nil {
		_ = stdin.Close()
		_ = session.Close()
		return nil, fmt.Errorf("failed to start account shell: %w", err)
	}

	account := newAccountShellSession(
		s.entry.Alias,
		opts.SessionID,
		targetUser,
		"bash",
		method,
		opts.Role,
		opts.Channel,
		s.currentCWD(),
		stdin,
		[]io.Reader{stdout, stderr},
		func() error {
			_ = session.Signal(ssh.SIGTERM)
			return session.Close()
		},
	)

	primeCtx, cancel := context.WithTimeout(ctx, defaultAccountShellPrimeTimeout)
	defer cancel()
	if _, err := account.Run(primeCtx, "true", opts, nil); err != nil {
		_ = account.Close()
		return nil, fmt.Errorf("prime account shell: %w", err)
	}
	return account, nil
}
