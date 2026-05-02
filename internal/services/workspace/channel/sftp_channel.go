package channel

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// sftpChannel multiplexes a small pool of *sftp.Client instances over the
// master SSH connection. It is pinned to PrivilegeDefault since SFTP runs as
// the connected user; per-privilege file access requires the caller to chain
// commands through an exec channel instead.
type sftpChannel struct {
	key    ChannelKey
	client *ssh.Client

	pool chan *sftp.Client

	mu       sync.Mutex
	state    ChannelState
	lastUsed time.Time
	closed   atomic.Bool
}

const sftpPoolSize = 4

func openSFTPChannel(client *ssh.Client, alias string) (*sftpChannel, error) {
	if client == nil {
		return nil, fmt.Errorf("channel: sftp requires a live ssh client")
	}
	c := &sftpChannel{
		key: ChannelKey{
			Alias:     alias,
			Privilege: PrivilegeDefault,
			Purpose:   PurposeSFTP,
		},
		client:   client,
		pool:     make(chan *sftp.Client, sftpPoolSize),
		state:    StateReady,
		lastUsed: time.Now(),
	}
	return c, nil
}

func (c *sftpChannel) Key() ChannelKey { return c.key }

func (c *sftpChannel) Snapshot() ChannelSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return ChannelSnapshot{
		Key:      c.key,
		State:    c.state,
		LastUsed: c.lastUsed,
	}
}

func (c *sftpChannel) acquire() (*sftp.Client, error) {
	select {
	case client := <-c.pool:
		if client != nil {
			return client, nil
		}
	default:
	}
	return sftp.NewClient(c.client)
}

func (c *sftpChannel) release(client *sftp.Client) {
	if client == nil {
		return
	}
	if c.closed.Load() {
		_ = client.Close()
		return
	}
	select {
	case c.pool <- client:
	default:
		_ = client.Close()
	}
}

func (c *sftpChannel) ReadFile(p string) ([]byte, error) {
	rc, err := c.OpenReader(p)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (c *sftpChannel) WriteFile(p string, content []byte, overwrite bool) error {
	wc, err := c.OpenWriter(p, overwrite)
	if err != nil {
		return err
	}
	if _, err := io.Copy(wc, bytes.NewReader(content)); err != nil {
		_ = wc.Close()
		return fmt.Errorf("write copy failed: %w", err)
	}
	if err := wc.Close(); err != nil {
		return err
	}

	// Best-effort size verification using a fresh client borrow.
	client, err := c.acquire()
	if err != nil {
		return nil
	}
	defer c.release(client)
	info, err := client.Stat(p)
	if err == nil && info.Size() != int64(len(content)) {
		return fmt.Errorf("size mismatch: wrote %d, expected %d", info.Size(), len(content))
	}
	return nil
}

func (c *sftpChannel) OpenReader(p string) (io.ReadCloser, error) {
	client, err := c.acquire()
	if err != nil {
		return nil, fmt.Errorf("open sftp client: %w", err)
	}
	file, err := client.Open(p)
	if err != nil {
		c.release(client)
		return nil, err
	}
	return &sftpReadCloser{file: file, release: func() { c.release(client) }}, nil
}

func (c *sftpChannel) OpenWriter(p string, overwrite bool) (io.WriteCloser, error) {
	client, err := c.acquire()
	if err != nil {
		return nil, fmt.Errorf("open sftp client: %w", err)
	}
	if !overwrite {
		if _, statErr := client.Stat(p); statErr == nil {
			c.release(client)
			return nil, fmt.Errorf("destination already exists: %s", p)
		}
	}
	dir := pathDir(p)
	if dir != "" && dir != "." {
		if _, err := client.Stat(dir); err != nil {
			if err := client.MkdirAll(dir); err != nil {
				c.release(client)
				return nil, fmt.Errorf("create remote directory %s: %w", dir, err)
			}
		}
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !overwrite {
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}
	file, err := client.OpenFile(p, flags)
	if err != nil {
		c.release(client)
		return nil, err
	}
	return &sftpWriteCloser{file: file, release: func() { c.release(client) }}, nil
}

func (c *sftpChannel) ListDir(root string, recursive bool, depth int) ([]FileEntry, error) {
	client, err := c.acquire()
	if err != nil {
		return nil, fmt.Errorf("open sftp client: %w", err)
	}
	defer c.release(client)

	base := strings.TrimSpace(root)
	if base == "" {
		base = "."
	}

	out := make([]FileEntry, 0, 64)
	var walk func(current string, currentDepth int) error
	walk = func(current string, currentDepth int) error {
		items, err := client.ReadDir(current)
		if err != nil {
			return err
		}
		for _, item := range items {
			itemPath := path.Join(current, item.Name())
			out = append(out, FileEntry{
				Name:    item.Name(),
				Path:    itemPath,
				IsDir:   item.IsDir(),
				Size:    item.Size(),
				Mode:    item.Mode().String(),
				ModTime: item.ModTime(),
			})
			if !recursive || !item.IsDir() {
				continue
			}
			nextDepth := currentDepth + 1
			if depth > 0 && nextDepth >= depth {
				continue
			}
			if err := walk(itemPath, nextDepth); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(base, 0); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Path < out[j].Path
	})
	return out, nil
}

func (c *sftpChannel) Stat(p string) (FileEntry, error) {
	client, err := c.acquire()
	if err != nil {
		return FileEntry{}, fmt.Errorf("open sftp client: %w", err)
	}
	defer c.release(client)
	info, err := client.Stat(p)
	if err != nil {
		return FileEntry{}, err
	}
	return FileEntry{
		Name:    info.Name(),
		Path:    p,
		IsDir:   info.IsDir(),
		Size:    info.Size(),
		Mode:    info.Mode().String(),
		ModTime: info.ModTime(),
	}, nil
}

func (c *sftpChannel) Remove(p string) error {
	client, err := c.acquire()
	if err != nil {
		return fmt.Errorf("open sftp client: %w", err)
	}
	defer c.release(client)
	return client.Remove(p)
}

func (c *sftpChannel) Mkdir(p string) error {
	client, err := c.acquire()
	if err != nil {
		return fmt.Errorf("open sftp client: %w", err)
	}
	defer c.release(client)
	return client.MkdirAll(p)
}

func (c *sftpChannel) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	c.mu.Lock()
	c.state = StateClosed
	c.mu.Unlock()
	for {
		select {
		case client := <-c.pool:
			if client != nil {
				_ = client.Close()
			}
		default:
			return nil
		}
	}
}

type sftpReadCloser struct {
	file    *sftp.File
	release func()
}

func (c *sftpReadCloser) Read(p []byte) (int, error) { return c.file.Read(p) }

func (c *sftpReadCloser) Close() error {
	fileErr := c.file.Close()
	if c.release != nil {
		c.release()
	}
	return fileErr
}

type sftpWriteCloser struct {
	file    *sftp.File
	release func()
}

func (c *sftpWriteCloser) Write(p []byte) (int, error) { return c.file.Write(p) }

func (c *sftpWriteCloser) Close() error {
	fileErr := c.file.Close()
	if c.release != nil {
		c.release()
	}
	return fileErr
}

func pathDir(p string) string {
	cleaned := strings.TrimSpace(p)
	if cleaned == "" {
		return ""
	}
	return path.Dir(cleaned)
}
