// Package ssh
// File: client.go
// Description: SSH 클라이언트 — 연결 수명 주기, 단일 명령 실행, 헬퍼 함수
// Responsibility: SSH 연결 관리(connect/close)와 비스트리밍 명령 실행(Run) 제공

package ssh

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/workspace"
	gossh "golang.org/x/crypto/ssh"
)

const defaultTimeout = 30 * time.Second

// Client represents an SSH client connection.
type Client struct {
	cfg       *Config
	conn      *gossh.Client
	mu        sync.Mutex
	mgr       *SessionManager
	lastError error
}

// RunResult captures the outcome of a non-streaming SSH command execution.
type RunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

// NewClient creates a new SSH client with the given configuration.
func NewClient(config *Config) *Client {
	cfg := *config
	cfg.WorkspaceDir = workspace.RemoteDirOrDefault(cfg.WorkspaceDir)
	return &Client{
		cfg: &cfg,
	}
}

// ensureConnected establishes the SSH connection if not already connected.
func (c *Client) ensureConnected(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return nil
	}

	port := c.cfg.Port
	if port == 0 {
		port = 22
	}
	addr := net.JoinHostPort(c.cfg.Host, fmt.Sprintf("%d", port))

	sshConfig, err := buildSSHConfig(c.cfg)
	if err != nil {
		return fmt.Errorf("ssh config: %w", err)
	}

	timeout := c.cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	dialer := net.Dialer{Timeout: timeout}
	netConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		c.lastError = err
		return fmt.Errorf("dial ssh %s: %w", addr, err)
	}

	clientConn, chans, reqs, err := gossh.NewClientConn(netConn, addr, sshConfig)
	if err != nil {
		_ = netConn.Close()
		c.lastError = err
		return fmt.Errorf("ssh new client conn %s: %w", addr, err)
	}
	c.conn = gossh.NewClient(clientConn, chans, reqs)
	c.lastError = nil
	return nil
}

// Close closes the SSH client connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

// LastError returns the last error encountered during connection attempts.
func (c *Client) LastError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastError
}

// Host returns the configured host (IP or hostname).
func (c *Client) Host() string {
	return c.cfg.Host
}

// Run executes a command on the remote host without streaming.
func (c *Client) Run(ctx context.Context, command string) (RunResult, error) {
	if err := c.ensureConnected(ctx); err != nil {
		return RunResult{}, err
	}

	session, err := c.newSession(ctx)
	if err != nil {
		return RunResult{}, err
	}
	defer session.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &executor.LimitedWriter{Buf: &stdoutBuf, Limit: executor.MaxOutputBytes}
	session.Stderr = &executor.LimitedWriter{Buf: &stderrBuf, Limit: executor.MaxOutputBytes}

	start := time.Now()
	runDone := make(chan error, 1)
	go func() {
		runDone <- session.Run(wrapLoginShell(command))
	}()

	var runErr error
	select {
	case runErr = <-runDone:
	case <-ctx.Done():
		_ = session.Close()
		runErr = <-runDone
		return RunResult{
			Stdout:   stdoutBuf.String(),
			Stderr:   stderrBuf.String(),
			ExitCode: 124,
			Duration: time.Since(start),
		}, fmt.Errorf("ssh run command cancelled: %w", ctx.Err())
	}
	if runErr != nil {
		if exitErr, ok := runErr.(*gossh.ExitError); ok {
			// Non-zero exit is an execution result, not a transport error.
			// Matches local executor contract: ExitError → ExecResult, no error returned.
			return RunResult{
				Stdout:   stdoutBuf.String(),
				Stderr:   stderrBuf.String(),
				ExitCode: exitErr.ExitStatus(),
				Duration: time.Since(start),
			}, nil
		}
		return RunResult{}, fmt.Errorf("ssh run command: %w", runErr)
	}

	return RunResult{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: 0,
		Duration: time.Since(start),
	}, nil
}

// newSession creates a new SSH session. Caller must not hold c.mu.
func (c *Client) newSession(ctx context.Context) (*gossh.Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		if c.lastError != nil {
			return nil, fmt.Errorf("ssh not connected (last error: %w)", c.lastError)
		}
		return nil, fmt.Errorf("ssh not connected")
	}

	session, err := c.conn.NewSession()
	if err != nil {
		return nil, fmt.Errorf("ssh new session: %w", err)
	}
	return session, nil
}

// SessionMgr returns the session manager for persistent shell sessions.
// The manager is created lazily on first access.
func (c *Client) SessionMgr() *SessionManager {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mgr == nil {
		c.mgr = newSessionManager(context.Background(), c)
	}
	return c.mgr
}

// ── helpers ──────────────────────────────────────────────────────────────────

// requestPTY allocates a pseudo-terminal on the SSH session.
// ECHO is disabled (0) so that passwords injected via stdin are not echoed back to stdout.
// All PTY-based execution in Infractl is agent-driven (no human typing), so echo is unnecessary.
func requestPTY(session *gossh.Session) error {
	modes := gossh.TerminalModes{
		gossh.ECHO:          0,
		gossh.TTY_OP_ISPEED: 14400,
		gossh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm", 80, 120, modes); err != nil {
		return fmt.Errorf("ssh request pty: %w", err)
	}
	return nil
}

// wrapLoginShell wraps the command with a login shell.
func wrapLoginShell(command string) string {
	if strings.HasPrefix(command, "sudo -i") || strings.HasPrefix(command, "su -") {
		return command
	}
	if command == "bash -l" || command == "sh -l" || command == "pwsh -l" {
		return command
	}
	return fmt.Sprintf("bash -l -c %s", sshQuote(command))
}

// sshQuote quotes a string for safe use within an SSH command.
// Single quotes are escaped using the standard POSIX idiom: ' → '"'"'
// (close quote, literal apostrophe in double-quotes, reopen quote).
// This preserves all special characters ($, !, @, #, spaces, parentheses)
// because the content of a single-quoted string is never interpreted by bash.
func sshQuote(s string) string {
	return `'` + strings.ReplaceAll(s, `'`, `'"'"'`) + `'`
}

// buildSSHConfig creates a gossh.ClientConfig from the application Config.
func buildSSHConfig(cfg *Config) (*gossh.ClientConfig, error) {
	sshCfg := &gossh.ClientConfig{
		User:            cfg.User,
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	sshCfg.Timeout = timeout

	switch cfg.AuthType {
	case "password":
		sshCfg.Auth = []gossh.AuthMethod{
			gossh.Password(cfg.Password),
		}
	case "key":
		keyBytes, err := os.ReadFile(cfg.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("read ssh key %s: %w", cfg.KeyPath, err)
		}
		var signer gossh.Signer
		if cfg.Password != "" {
			signer, err = gossh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(cfg.Password))
		} else {
			signer, err = gossh.ParsePrivateKey(keyBytes)
		}
		if err != nil {
			return nil, fmt.Errorf("parse ssh key %s: %w", cfg.KeyPath, err)
		}
		sshCfg.Auth = []gossh.AuthMethod{
			gossh.PublicKeys(signer),
		}
	default:
		return nil, fmt.Errorf("unsupported auth type: %s", cfg.AuthType)
	}
	return sshCfg, nil
}
