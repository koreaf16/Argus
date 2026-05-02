//go:build !windows

package workspace

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"strings"

	"github.com/creack/pty"
	"github.com/koreaf16/argus/internal/utils"
)

func (m *Manager) openLocalAccountShell(ctx context.Context, opts ExecOptions) (*accountShellSession, error) {
	_ = ctx

	currentUser := os.Getenv("USER")
	if currentUser == "" {
		if u, err := user.Current(); err == nil {
			currentUser = u.Username
		}
	}
	targetUser := strings.TrimSpace(opts.AsUser)
	if targetUser == "" {
		targetUser = currentUser
	}

	method := normalizePrivilegeMethod(opts.PrivilegeMethod)
	var cmd *exec.Cmd
	if targetUser != "" && !strings.EqualFold(targetUser, currentUser) {
		switch method {
		case PrivilegeSU:
			cmd = exec.Command("su", "-s", "/bin/bash", "-", targetUser)
		default:
			method = PrivilegeSudo
			cmd = exec.Command("sudo", "-u", targetUser, "bash", "-li")
		}
	} else {
		method = PrivilegeNone
		shellPath, err := utils.FindSuitableShell()
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(shellPath) == "" {
			shellPath = "bash"
		}
		cmd = exec.Command(shellPath, "-li")
	}
	cmd.Env = append(os.Environ(), "PAGER=cat", "SYSTEMD_PAGER=cat")

	f, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("start local account shell: %w", err)
	}

	account := newAccountShellSession(
		LocalAlias,
		opts.SessionID,
		targetUser,
		"bash",
		method,
		opts.Role,
		opts.Channel,
		"",
		f,
		[]io.Reader{f},
		func() error {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return f.Close()
		},
	)

	primeCtx, cancel := context.WithTimeout(ctx, defaultAccountShellPrimeTimeout)
	defer cancel()
	if _, err := account.Run(primeCtx, "true", opts, nil); err != nil {
		_ = account.Close()
		return nil, fmt.Errorf("prime local account shell: %w", err)
	}
	return account, nil
}
