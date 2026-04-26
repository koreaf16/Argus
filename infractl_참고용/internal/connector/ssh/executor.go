// Package ssh
// File: executor.go
// Description: SSH-backed executor implementation bridged to the shared executor interfaces.

package ssh

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/serverenv"
)

// SSHExecutor executes commands on a remote target via SSH.
type SSHExecutor struct {
	name         string
	client       *Client
	platform     executor.Platform
	workspaceDir string
}

// NewSSHExecutor wraps an SSH client with an executor implementation.
func NewSSHExecutor(name string, client *Client, osHint ...string) *SSHExecutor {
	platform := executor.PlatformLinux
	if len(osHint) > 0 {
		if detected := executor.NormalizePlatform(osHint[0]); detected != executor.PlatformUnknown {
			platform = detected
		}
	}
	workspaceDir := client.cfg.WorkspaceDir
	if len(osHint) > 1 && strings.TrimSpace(osHint[1]) != "" {
		workspaceDir = osHint[1]
	}
	return &SSHExecutor{
		name:         name,
		client:       client,
		platform:     platform,
		workspaceDir: serverenv.RemoteDirOrDefault(workspaceDir),
	}
}

// Execute runs a command on the remote host.
func (e *SSHExecutor) Execute(ctx context.Context, command string) (executor.ExecResult, error) {
	result, err := e.client.Run(ctx, e.workspaceCommand(command))
	if err != nil {
		return executor.ExecResult{}, fmt.Errorf("ssh execute on %s: %w", e.name, err)
	}

	return executor.ExecResult{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
		Duration: result.Duration,
	}, nil
}

// ExecuteStream runs a command and streams stdout line-by-line.
func (e *SSHExecutor) ExecuteStream(ctx context.Context, command string, onLine func(string)) (executor.ExecSession, error) {
	return e.client.RunStream(ctx, e.workspaceCommand(command), false, onLine, nil)
}

// ExecuteStreamPTY runs a command with PTY allocated, streaming stdout line-by-line.
// PTY ensures interactive prompts (sudo, su, passwd) appear in stdout
// instead of /dev/tty, enabling the idle handler to detect and auto-respond.
func (e *SSHExecutor) ExecuteStreamPTY(ctx context.Context, command string, onLine func(string)) (executor.ExecSession, error) {
	return e.client.RunStream(ctx, e.workspaceCommand(command), true, onLine, nil)
}

// ExecuteInteractive runs a command while streaming raw terminal chunks.
func (e *SSHExecutor) ExecuteInteractive(ctx context.Context, spec executor.InteractiveSpec, onChunk func(string)) (executor.ExecSession, error) {
	spec.Command = e.workspaceCommand(spec.Command)
	return e.client.RunInteractive(ctx, spec.Command, spec.RequirePTY, onChunk)
}

// ExecuteStreamWithIdle runs a command and exposes idle callbacks for interactive prompts.
func (e *SSHExecutor) ExecuteStreamWithIdle(
	ctx context.Context,
	command string,
	onLine func(string),
	onIdle func(executor.StdinInjector),
) (executor.ExecSession, error) {
	return e.client.RunStream(ctx, e.workspaceCommand(command), false, onLine, onIdle)
}

// Target returns the server alias.
func (e *SSHExecutor) Target() string {
	return e.name
}

// Host returns the remote IP/host.
func (e *SSHExecutor) Host() string {
	return e.client.Host()
}

func (e *SSHExecutor) WorkspaceDir() string {
	return serverenv.RemoteDirOrDefault(e.workspaceDir)
}

// Platform returns the remote target platform.
func (e *SSHExecutor) Platform() executor.Platform {
	return e.platform
}

// ShellName returns the preferred shell label for the remote target.
func (e *SSHExecutor) ShellName() string {
	if e.platform == executor.PlatformWindows {
		return "PowerShell"
	}
	return "bash"
}

// Upload transfers a local file to the remote host via SFTP.
// Implements executor.FileTransferExecutor.
func (e *SSHExecutor) Upload(ctx context.Context, localPath, remotePath string, onProgress func(transferred, total int64)) error {
	resolved, err := e.resolveTransferPath(ctx, remotePath)
	if err != nil {
		return fmt.Errorf("resolve remote upload path %s: %w", remotePath, err)
	}
	remotePath = resolved
	if err := e.client.Upload(ctx, localPath, remotePath, onProgress); err != nil {
		return fmt.Errorf("sftp upload to %s: %w", e.name, err)
	}
	return nil
}

// Download transfers a file from the remote host to a local path via SFTP.
// Implements executor.FileTransferExecutor.
func (e *SSHExecutor) Download(ctx context.Context, remotePath, localPath string, onProgress func(transferred, total int64)) error {
	resolved, err := e.resolveTransferPath(ctx, remotePath)
	if err != nil {
		return fmt.Errorf("resolve remote download path %s: %w", remotePath, err)
	}
	remotePath = resolved
	if err := e.client.Download(ctx, remotePath, localPath, onProgress); err != nil {
		return fmt.Errorf("sftp download from %s: %w", e.name, err)
	}
	return nil
}

// Close closes the SSH client connection.
func (e *SSHExecutor) Close() error {
	return e.client.Close()
}

func (e *SSHExecutor) workspaceCommand(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return command
	}
	ws := serverenv.POSIXShellPath(e.WorkspaceDir())
	return fmt.Sprintf("mkdir -p %s && cd %s && %s", ws, ws, command)
}

func (e *SSHExecutor) resolveTransferPath(ctx context.Context, remotePath string) (string, error) {
	remotePath = strings.TrimSpace(remotePath)
	if remotePath == "" || path.IsAbs(remotePath) {
		return remotePath, nil
	}

	ws := serverenv.POSIXShellPath(e.WorkspaceDir())
	pathExpr := serverenv.POSIXShellPath(remotePath)
	script := fmt.Sprintf(
		`mkdir -p %s && cd %s && target=%s && case "$target" in /*) printf '%%s' "$target" ;; *) printf '%%s' "$PWD/$target" ;; esac`,
		ws,
		ws,
		pathExpr,
	)
	result, err := e.client.Run(ctx, script)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("remote path resolve failed: %s", strings.TrimSpace(result.Stderr))
	}
	resolved := strings.TrimSpace(result.Stdout)
	if resolved == "" {
		return remotePath, nil
	}
	return resolved, nil
}

// ── PersistentSessionExecutor ────────────────────────────────────────────────

// SessionExecute runs command within the named persistent shell session.
// The session is created automatically on first use (starts as the SSH login user).
// onIdle is called when output stalls; returning ("", true) aborts the command.
func (e *SSHExecutor) SessionExecute(
	ctx context.Context,
	sessionID, command string,
	timeout time.Duration,
	onIdle func([]string) (string, bool),
) (executor.ShellRunResult, error) {
	sh, err := e.client.SessionMgr().GetOrCreate(ctx, sessionID)
	if err != nil {
		return executor.ShellRunResult{}, fmt.Errorf("session execute on %s: %w", e.name, err)
	}
	result, err := sh.RunCommand(ctx, command, timeout, onIdle)
	if err != nil {
		return executor.ShellRunResult{}, fmt.Errorf("session execute on %s: %w", e.name, err)
	}
	return result, nil
}

// SessionElevate runs elevationCmd (e.g., "sudo -i", "su - oracle") in the named session
// without delimiter wrapping so the elevated shell persists for subsequent SessionExecute calls.
func (e *SSHExecutor) SessionElevate(
	ctx context.Context,
	sessionID, elevationCmd string,
	timeout time.Duration,
	onIdle func([]string) (string, bool),
) (executor.ShellRunResult, error) {
	if err := e.client.SessionMgr().Elevate(ctx, sessionID, elevationCmd, timeout, onIdle); err != nil {
		return executor.ShellRunResult{}, fmt.Errorf("session elevate on %s: %w", e.name, err)
	}
	// Return current session state as result
	if info, ok := e.client.SessionMgr().Info(sessionID); ok {
		return executor.ShellRunResult{
			Stdout:      "",
			ExitCode:    0,
			CurrentUser: info.CurrentUser,
			CurrentDir:  info.CurrentDir,
		}, nil
	}
	return executor.ShellRunResult{ExitCode: 0}, nil
}

// SessionClose destroys the named session and releases its SSH channel.
func (e *SSHExecutor) SessionClose(ctx context.Context, sessionID string) error {
	return e.client.SessionMgr().Close(sessionID)
}

// SessionList returns metadata for all active persistent sessions on this target.
func (e *SSHExecutor) SessionList(ctx context.Context) ([]executor.SessionInfo, error) {
	return e.client.SessionMgr().ListAll(), nil
}
