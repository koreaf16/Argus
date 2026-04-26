// Package ssh
// File: sftp.go
// Description: SFTP 파일 전송 — 기존 SSH 연결을 재사용해 패스워드 없이 업로드/다운로드
// Responsibility: 인증된 SSH 연결 위에서 SFTP 세션을 열고 파일을 전송한다

package ssh

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"

	"github.com/pkg/sftp"
)

// Upload transfers a local file to the remote host via SFTP.
// Reuses the existing authenticated SSH connection — no password prompt.
// Falls back to SCP wire protocol if the remote SFTP subsystem is unavailable.
// onProgress may be nil; when set it is called with (transferred, total) after each chunk.
func (c *Client) Upload(ctx context.Context, localPath, remotePath string, onProgress func(transferred, total int64)) error {
	if err := c.ensureConnected(ctx); err != nil {
		return fmt.Errorf("ssh connect for sftp: %w", err)
	}

	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	sc, err := sftp.NewClient(conn)
	if err != nil {
		slog.Info("sftp subsystem unavailable, falling back to scp", "host", c.cfg.Host, "err", err)
		if scpErr := uploadSCP(ctx, conn, localPath, remotePath, onProgress); scpErr != nil {
			slog.Info("scp also unavailable, falling back to shell cat", "host", c.cfg.Host, "err", scpErr)
			if catErr := uploadShellCat(ctx, conn, localPath, remotePath, onProgress); catErr != nil {
				return fmt.Errorf("all transfer methods failed (sftp: %v, scp: %v): %w", err, scpErr, catErr)
			}
			return nil
		}
		return nil
	}
	defer sc.Close()

	src, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local file %s: %w", localPath, err)
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return fmt.Errorf("stat local file %s: %w", localPath, err)
	}
	total := info.Size()

	remoteDir := path.Dir(remotePath)
	if mkErr := sc.MkdirAll(remoteDir); mkErr != nil {
		slog.Debug("sftp mkdirall failed (may already exist)", "dir", remoteDir, "err", mkErr)
	}

	dst, err := sc.Create(remotePath)
	if err != nil {
		return fmt.Errorf("sftp create %s: %w", remotePath, err)
	}
	defer dst.Close()

	transferred, err := copyWithProgress(ctx, dst, src, total, onProgress)
	if err != nil {
		return fmt.Errorf("sftp upload %s → %s: transferred %d bytes: %w", localPath, remotePath, transferred, err)
	}
	slog.Info("sftp upload complete", "local", localPath, "remote", remotePath, "bytes", transferred)
	return nil
}

// Download transfers a file from the remote host to a local path via SFTP.
// Reuses the existing authenticated SSH connection — no password prompt.
// Falls back to SCP wire protocol if the remote SFTP subsystem is unavailable.
func (c *Client) Download(ctx context.Context, remotePath, localPath string, onProgress func(transferred, total int64)) error {
	if err := c.ensureConnected(ctx); err != nil {
		return fmt.Errorf("ssh connect for sftp: %w", err)
	}

	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	sc, err := sftp.NewClient(conn)
	if err != nil {
		slog.Info("sftp subsystem unavailable, falling back to scp", "host", c.cfg.Host, "err", err)
		if scpErr := downloadSCP(ctx, conn, remotePath, localPath, onProgress); scpErr != nil {
			slog.Info("scp also unavailable, falling back to shell cat", "host", c.cfg.Host, "err", scpErr)
			if catErr := downloadShellCat(ctx, conn, remotePath, localPath, onProgress); catErr != nil {
				return fmt.Errorf("all transfer methods failed (sftp: %v, scp: %v): %w", err, scpErr, catErr)
			}
			return nil
		}
		return nil
	}
	defer sc.Close()

	src, err := sc.Open(remotePath)
	if err != nil {
		return fmt.Errorf("sftp open %s: %w", remotePath, err)
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return fmt.Errorf("sftp stat %s: %w", remotePath, err)
	}
	total := info.Size()

	if mkErr := os.MkdirAll(filepath.Dir(localPath), 0o755); mkErr != nil {
		return fmt.Errorf("mkdir local %s: %w", filepath.Dir(localPath), mkErr)
	}

	dst, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create local file %s: %w", localPath, err)
	}
	defer dst.Close()

	transferred, err := copyWithProgress(ctx, dst, src, total, onProgress)
	if err != nil {
		return fmt.Errorf("sftp download %s → %s: transferred %d bytes: %w", remotePath, localPath, transferred, err)
	}
	slog.Info("sftp download complete", "remote", remotePath, "local", localPath, "bytes", transferred)
	return nil
}

// copyWithProgress copies from src to dst in 32 KiB chunks, reporting progress via onProgress.
func copyWithProgress(ctx context.Context, dst io.Writer, src io.Reader, total int64, onProgress func(int64, int64)) (int64, error) {
	buf := make([]byte, 32*1024)
	var transferred int64
	for {
		if ctx.Err() != nil {
			return transferred, fmt.Errorf("cancelled: %w", ctx.Err())
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return transferred, writeErr
			}
			transferred += int64(n)
			if onProgress != nil {
				onProgress(transferred, total)
			}
		}
		if readErr == io.EOF {
			return transferred, nil
		}
		if readErr != nil {
			return transferred, readErr
		}
	}
}
