// Package ssh
// File: scp.go
// Description: SCP wire protocol transfer — fallback when SFTP subsystem is unavailable.
// Responsibility: Upload/download files using SCP protocol over an existing SSH session.

package ssh

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	gossh "golang.org/x/crypto/ssh"
)

// uploadSCP transfers localPath to remotePath using SCP sink protocol.
// Runs "scp -qt <remotePath>" on the remote side over the existing SSH connection.
// The remote directory is created with best-effort mkdir before transfer.
func uploadSCP(ctx context.Context, conn *gossh.Client, localPath, remotePath string, onProgress func(int64, int64)) error {
	// Best-effort remote directory creation
	if mkErr := runSimpleCommand(conn, "mkdir -p "+scpQuote(path.Dir(remotePath))); mkErr != nil {
		slog.Debug("scp mkdir failed", "dir", path.Dir(remotePath), "err", mkErr)
	}

	session, err := conn.NewSession()
	if err != nil {
		return fmt.Errorf("scp new session: %w", err)
	}
	defer session.Close()

	var stderrBuf bytes.Buffer
	session.Stderr = &stderrBuf

	stdinPipe, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("scp stdin pipe: %w", err)
	}
	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("scp stdout pipe: %w", err)
	}

	if err := session.Start("scp -qt " + scpQuote(remotePath)); err != nil {
		return fmt.Errorf("scp start: %w", err)
	}

	r := bufio.NewReader(stdoutPipe)

	// Remote sink sends a null byte when ready.
	if err := scpCheckAck(r); err != nil {
		return fmt.Errorf("scp ready signal: %w", err)
	}

	src, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", localPath, err)
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", localPath, err)
	}
	size := info.Size()

	// Use remote basename so the file lands at the exact remotePath.
	if _, err := fmt.Fprintf(stdinPipe, "C0644 %d %s\n", size, filepath.Base(remotePath)); err != nil {
		return fmt.Errorf("scp header write: %w", err)
	}
	if err := scpCheckAck(r); err != nil {
		return fmt.Errorf("scp header ack: %w", err)
	}

	if _, err := copyWithProgress(ctx, stdinPipe, src, size, onProgress); err != nil {
		return fmt.Errorf("scp content write: %w", err)
	}

	// End-of-content marker
	if _, err := stdinPipe.Write([]byte{0}); err != nil {
		return fmt.Errorf("scp eof marker: %w", err)
	}
	if err := scpCheckAck(r); err != nil {
		return fmt.Errorf("scp final ack: %w", err)
	}

	stdinPipe.Close()
	if waitErr := session.Wait(); waitErr != nil {
		if ex, ok := waitErr.(*gossh.ExitError); ok && ex.ExitStatus() == 0 {
			return nil
		}
		// Ignore non-zero exit if we already got all acks; some servers exit 1 on clean transfer.
		slog.Debug("scp session exit", "err", waitErr, "stderr", stderrBuf.String())
	}
	return nil
}

// downloadSCP fetches remotePath to localPath using SCP source protocol.
// Runs "scp -qf <remotePath>" on the remote side over the existing SSH connection.
func downloadSCP(ctx context.Context, conn *gossh.Client, remotePath, localPath string, onProgress func(int64, int64)) error {
	session, err := conn.NewSession()
	if err != nil {
		return fmt.Errorf("scp new session: %w", err)
	}
	defer session.Close()

	var stderrBuf bytes.Buffer
	session.Stderr = &stderrBuf

	stdinPipe, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("scp stdin pipe: %w", err)
	}
	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("scp stdout pipe: %w", err)
	}

	if err := session.Start("scp -qf " + scpQuote(remotePath)); err != nil {
		return fmt.Errorf("scp start: %w", err)
	}

	r := bufio.NewReader(stdoutPipe)

	// Send null byte to trigger the remote source to start sending.
	if _, err := stdinPipe.Write([]byte{0}); err != nil {
		return fmt.Errorf("scp trigger: %w", err)
	}

	// Read file header: "C<mode> <size> <name>\n"
	header, err := r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("scp read header: %w", err)
	}
	header = strings.TrimRight(header, "\n\r")

	switch {
	case len(header) == 0:
		return fmt.Errorf("scp: empty header from remote")
	case header[0] == 0x01 || header[0] == 0x02:
		return fmt.Errorf("scp remote error: %s", header[1:])
	case header[0] != 'C':
		return fmt.Errorf("scp unexpected header: 0x%02x %q", header[0], header)
	}

	// Parse "C<mode> <size> <name>"
	fields := strings.SplitN(header[1:], " ", 3)
	if len(fields) != 3 {
		return fmt.Errorf("scp malformed header: %q", header)
	}
	size, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return fmt.Errorf("scp parse size %q: %w", fields[1], err)
	}

	// ACK the header.
	if _, err := stdinPipe.Write([]byte{0}); err != nil {
		return fmt.Errorf("scp ack header: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return fmt.Errorf("scp mkdir local: %w", err)
	}
	dst, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("scp create local: %w", err)
	}
	defer dst.Close()

	// Read exactly <size> bytes.
	if _, err := copyWithProgress(ctx, dst, io.LimitReader(r, size), size, onProgress); err != nil {
		return fmt.Errorf("scp read content: %w", err)
	}

	// Read the trailing null byte sent after file data.
	trail := make([]byte, 1)
	if _, err := r.Read(trail); err != nil {
		return fmt.Errorf("scp read trailer: %w", err)
	}
	if trail[0] != 0 {
		return fmt.Errorf("scp unexpected trailer byte: 0x%02x", trail[0])
	}

	// ACK completion.
	if _, err := stdinPipe.Write([]byte{0}); err != nil {
		return fmt.Errorf("scp ack completion: %w", err)
	}

	stdinPipe.Close()
	if waitErr := session.Wait(); waitErr != nil {
		if ex, ok := waitErr.(*gossh.ExitError); ok && ex.ExitStatus() == 0 {
			return nil
		}
		slog.Debug("scp session exit", "err", waitErr, "stderr", stderrBuf.String())
	}
	return nil
}

// scpCheckAck reads one byte from the SCP control channel.
// 0x00 = success; 0x01/0x02 = warning/error followed by a message line.
func scpCheckAck(r *bufio.Reader) error {
	b, err := r.ReadByte()
	if err != nil {
		return fmt.Errorf("read ack: %w", err)
	}
	switch b {
	case 0:
		return nil
	case 1, 2:
		msg, _ := r.ReadString('\n')
		return fmt.Errorf("remote: %s", strings.TrimRight(msg, "\n"))
	default:
		return fmt.Errorf("unexpected ack byte: 0x%02x", b)
	}
}

// scpQuote wraps arg in single quotes for safe shell execution,
// escaping any embedded single quotes using the '\'' idiom.
func scpQuote(arg string) string {
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

// runSimpleCommand opens a short-lived session to run a single command, ignoring its output.
func runSimpleCommand(conn *gossh.Client, cmd string) error {
	session, err := conn.NewSession()
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}
	defer session.Close()
	if err := session.Run(cmd); err != nil {
		return fmt.Errorf("run %q: %w", cmd, err)
	}
	return nil
}

// runCommandOutput opens a short-lived session and returns the command's stdout.
func runCommandOutput(conn *gossh.Client, cmd string) (string, error) {
	session, err := conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer session.Close()
	var buf bytes.Buffer
	session.Stdout = &buf
	if err := session.Run(cmd); err != nil {
		return "", fmt.Errorf("run %q: %w", cmd, err)
	}
	return buf.String(), nil
}
