// Package ssh
// File: shellcat.go
// Description: Shell-cat 전송 — SFTP/SCP 모두 없는 서버용 최후 폴백
// Responsibility: cat으로 파일을 stdin 파이프로 전송한다 (POSIX 서버 범용 호환)

package ssh

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	gossh "golang.org/x/crypto/ssh"
)

// uploadShellCat transfers localPath to remotePath using "cat > file" over SSH stdin.
// Last-resort fallback when neither the SFTP subsystem nor the remote scp binary is available.
// Works on any POSIX host that has cat (universally available).
func uploadShellCat(ctx context.Context, conn *gossh.Client, localPath, remotePath string, onProgress func(int64, int64)) error {
	if mkErr := runSimpleCommand(conn, "mkdir -p "+scpQuote(path.Dir(remotePath))); mkErr != nil {
		slog.Debug("shellcat mkdir failed", "dir", path.Dir(remotePath), "err", mkErr)
	}

	session, err := conn.NewSession()
	if err != nil {
		return fmt.Errorf("shellcat new session: %w", err)
	}
	defer session.Close()

	var stderrBuf bytes.Buffer
	session.Stderr = &stderrBuf

	stdinPipe, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("shellcat stdin pipe: %w", err)
	}

	if err := session.Start("cat > " + scpQuote(remotePath)); err != nil {
		return fmt.Errorf("shellcat start: %w", err)
	}

	src, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("shellcat open %s: %w", localPath, err)
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return fmt.Errorf("shellcat stat %s: %w", localPath, err)
	}

	transferred, err := copyWithProgress(ctx, stdinPipe, src, info.Size(), onProgress)
	if err != nil {
		return fmt.Errorf("shellcat copy %s → %s: transferred %d bytes: %w", localPath, remotePath, transferred, err)
	}

	stdinPipe.Close()
	if waitErr := session.Wait(); waitErr != nil {
		if ex, ok := waitErr.(*gossh.ExitError); ok && ex.ExitStatus() == 0 {
			return nil
		}
		return fmt.Errorf("shellcat upload wait: %w (stderr: %s)", waitErr, strings.TrimSpace(stderrBuf.String()))
	}

	slog.Info("shellcat upload complete", "local", localPath, "remote", remotePath, "bytes", transferred)
	return nil
}

// downloadShellCat fetches remotePath to localPath using "cat file" over SSH stdout.
// Last-resort fallback when neither the SFTP subsystem nor the remote scp binary is available.
func downloadShellCat(ctx context.Context, conn *gossh.Client, remotePath, localPath string, onProgress func(int64, int64)) error {
	// Best-effort size probe for accurate progress reporting.
	var total int64
	if out, sizeErr := runCommandOutput(conn, "stat -c %s "+scpQuote(remotePath)); sizeErr == nil {
		if n, parseErr := strconv.ParseInt(strings.TrimSpace(out), 10, 64); parseErr == nil {
			total = n
		}
	}

	session, err := conn.NewSession()
	if err != nil {
		return fmt.Errorf("shellcat download new session: %w", err)
	}
	defer session.Close()

	var stderrBuf bytes.Buffer
	session.Stderr = &stderrBuf

	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("shellcat download stdout pipe: %w", err)
	}

	if err := session.Start("cat " + scpQuote(remotePath)); err != nil {
		return fmt.Errorf("shellcat download start: %w", err)
	}

	if mkErr := os.MkdirAll(filepath.Dir(localPath), 0o755); mkErr != nil {
		return fmt.Errorf("shellcat mkdir local: %w", mkErr)
	}

	dst, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("shellcat create local %s: %w", localPath, err)
	}
	defer dst.Close()

	transferred, err := copyWithProgress(ctx, dst, stdoutPipe, total, onProgress)
	if err != nil {
		return fmt.Errorf("shellcat download copy: %w", err)
	}

	if waitErr := session.Wait(); waitErr != nil {
		if ex, ok := waitErr.(*gossh.ExitError); ok && ex.ExitStatus() == 0 {
			return nil
		}
		return fmt.Errorf("shellcat download wait: %w (stderr: %s)", waitErr, strings.TrimSpace(stderrBuf.String()))
	}

	slog.Info("shellcat download complete", "remote", remotePath, "local", localPath, "bytes", transferred)
	return nil
}
