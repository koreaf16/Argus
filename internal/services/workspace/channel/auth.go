package channel

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// EntrySpec is the channel package's lite view of a workspace.ServerEntry.
// It captures exactly the fields required to dial a master SSH connection.
// Using a local type avoids an import cycle (workspace -> channel -> workspace).
type EntrySpec struct {
	Alias         string
	Host          string
	Port          int
	User          string
	DefaultCWD    string
	IdentityFile  string
	UseAgent      bool
	AllowPassword bool
}

// PasswordPrompt is the channel-package equivalent of workspace.PasswordRequestFunc.
// The pool wraps the workspace function so the channel package does not need to
// import the manager.
type PasswordPrompt func(alias, kind, prompt string) (string, error)

// AuthFailedError reports a recoverable authentication failure (typically
// wrong password). Callers retry by clearing the cached password and
// invoking PasswordPrompt again.
type AuthFailedError struct{ Msg string }

func (e *AuthFailedError) Error() string { return e.Msg }

// IsAuthFailed reports whether the error chain contains an AuthFailedError.
func IsAuthFailed(err error) bool {
	var afe *AuthFailedError
	return errors.As(err, &afe)
}

// BuildAuthMethods constructs the ssh.AuthMethod chain for an entry. Returned
// closers must be closed when the *ssh.Client is closed (the agent socket is
// the typical example).
func BuildAuthMethods(entry EntrySpec, prompt PasswordPrompt) ([]ssh.AuthMethod, []io.Closer, error) {
	methods := make([]ssh.AuthMethod, 0, 4)
	closers := make([]io.Closer, 0, 1)

	if entry.UseAgent {
		if sock := strings.TrimSpace(os.Getenv("SSH_AUTH_SOCK")); sock != "" {
			conn, err := net.Dial("unix", sock)
			if err == nil {
				agentClient := agent.NewClient(conn)
				signers, signerErr := agentClient.Signers()
				if signerErr == nil && len(signers) > 0 {
					methods = append(methods, ssh.PublicKeys(signers...))
					closers = append(closers, conn)
				} else {
					_ = conn.Close()
				}
			}
		}
	}

	identityFile := strings.TrimSpace(entry.IdentityFile)
	if identityFile != "" {
		signer, err := loadSigner(identityFile, entry.Alias, prompt)
		if err != nil {
			closeAll(closers)
			return nil, nil, err
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	if entry.AllowPassword && prompt != nil {
		methods = append(methods, ssh.PasswordCallback(func() (string, error) {
			return prompt(entry.Alias, "ssh", "SSH password:")
		}))
		methods = append(methods, ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i, q := range questions {
				pw, err := prompt(entry.Alias, "ssh", q)
				if err != nil {
					return nil, err
				}
				answers[i] = pw
			}
			return answers, nil
		}))
	}

	return methods, closers, nil
}

func loadSigner(identityFile, alias string, prompt PasswordPrompt) (ssh.Signer, error) {
	keyBytes, err := os.ReadFile(identityFile)
	if err != nil {
		return nil, fmt.Errorf("read identity file %s for %s: %w", identityFile, alias, err)
	}
	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err == nil {
		return signer, nil
	}

	var passphraseErr *ssh.PassphraseMissingError
	if !errors.As(err, &passphraseErr) {
		return nil, fmt.Errorf("parse identity file %s for %s: %w", identityFile, alias, err)
	}
	if prompt == nil {
		return nil, fmt.Errorf("identity %s requires passphrase for %s, but no prompt callback is configured", identityFile, alias)
	}

	passphrase, promptErr := prompt(alias, "ssh", "SSH key passphrase:")
	if promptErr != nil {
		return nil, promptErr
	}
	signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(passphrase))
	if err != nil {
		return nil, fmt.Errorf("parse encrypted identity file %s for %s: %w", identityFile, alias, err)
	}
	return signer, nil
}

func closeAll(closers []io.Closer) {
	for _, c := range closers {
		_ = c.Close()
	}
}

// KnownHostsPath returns the project-specific known_hosts file path under
// ~/.Argus. Falls back to "./.Argus/ssh_known_hosts" if the home directory
// cannot be resolved.
func KnownHostsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".Argus", "ssh_known_hosts")
	}
	return filepath.Join(home, ".Argus", "ssh_known_hosts")
}

// TOFUHostKeyCallback returns a TOFU (trust-on-first-use) host key callback
// that records new host keys to knownHostsFile and rejects mismatched keys to
// surface MITM attempts.
func TOFUHostKeyCallback(knownHostsFile string) (ssh.HostKeyCallback, error) {
	if err := os.MkdirAll(filepath.Dir(knownHostsFile), 0o700); err != nil {
		return nil, fmt.Errorf("create known_hosts dir: %w", err)
	}
	f, err := os.OpenFile(knownHostsFile, os.O_CREATE|os.O_RDONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open known_hosts: %w", err)
	}
	_ = f.Close()

	cb, err := knownhosts.New(knownHostsFile)
	if err != nil {
		return nil, fmt.Errorf("parse known_hosts: %w", err)
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := cb(hostname, remote, key)
		if err == nil {
			return nil
		}
		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			return err
		}
		if len(keyErr.Want) > 0 {
			return fmt.Errorf(
				"SSH host key changed for %s. Possible MITM. Remove the previous entry from %s and retry",
				hostname, knownHostsFile,
			)
		}
		wf, werr := os.OpenFile(knownHostsFile, os.O_APPEND|os.O_WRONLY, 0o600)
		if werr != nil {
			return fmt.Errorf("record host key (%s) failed: %w", hostname, werr)
		}
		defer wf.Close()
		line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
		if _, werr := fmt.Fprintln(wf, line); werr != nil {
			return fmt.Errorf("write host key (%s) failed: %w", hostname, werr)
		}
		return nil
	}, nil
}

// classifyDialError converts a raw ssh.Dial error into a more actionable
// message and flags authentication failures so the pool can prompt for a new
// password.
func classifyDialError(err error, addr, host string) error {
	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "unable to authenticate") && strings.Contains(errStr, "password"):
		return &AuthFailedError{Msg: "authentication failed: incorrect password or password auth disabled"}
	case strings.Contains(errStr, "unable to authenticate") && strings.Contains(errStr, "publickey"):
		return fmt.Errorf("authentication failed: invalid SSH key or key rejected by server")
	case strings.Contains(errStr, "connection refused"):
		return fmt.Errorf("connection refused: check if host/port (%s) is correct and SSH is running", addr)
	case strings.Contains(errStr, "no such host"):
		return fmt.Errorf("host not found: invalid address %q", host)
	case strings.Contains(errStr, "i/o timeout"):
		return fmt.Errorf("connection timeout: server is unreachable")
	}
	return fmt.Errorf("ssh dial failed: %v", err)
}
