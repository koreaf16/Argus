package channel

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

// Connection wraps a single *ssh.Client for one alias. All channels for that
// alias multiplex over this one client. If the client dies, every channel is
// invalidated and the next ConnectionPool.Get reopens both client and channels.
type Connection struct {
	alias     string
	entry     EntrySpec
	client    *ssh.Client
	loginUser string

	mu       sync.Mutex
	channels map[ChannelKey]Channel

	authClosers []io.Closer
	closed      atomic.Bool
}

// dialConnection establishes a master SSH connection. It does not open any
// channels — the first AcquireExec/AcquireSFTP/etc. on the returned Connection
// is responsible for creating the underlying SSH session.
func dialConnection(ctx context.Context, entry EntrySpec, prompt PasswordPrompt) (*Connection, error) {
	authMethods, closers, err := BuildAuthMethods(entry, prompt)
	if err != nil {
		return nil, err
	}
	if len(authMethods) == 0 {
		closeAll(closers)
		return nil, fmt.Errorf("no SSH auth method is available for %s", entry.Alias)
	}
	hostKeyCB, err := TOFUHostKeyCallback(KnownHostsPath())
	if err != nil {
		closeAll(closers)
		return nil, fmt.Errorf("build host key callback: %w", err)
	}

	port := entry.Port
	if port <= 0 {
		port = 22
	}
	addr := net.JoinHostPort(entry.Host, strconv.Itoa(port))

	cfg := &ssh.ClientConfig{
		User:            entry.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCB,
		Timeout:         30 * time.Second,
	}

	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		closeAll(closers)
		return nil, classifyDialError(err, addr, entry.Host)
	}

	return &Connection{
		alias:       entry.Alias,
		entry:       entry,
		client:      client,
		loginUser:   entry.User,
		channels:    make(map[ChannelKey]Channel),
		authClosers: closers,
	}, nil
}

// Alias returns the host alias.
func (c *Connection) Alias() string { return c.alias }

// Entry returns a copy of the underlying EntrySpec.
func (c *Connection) Entry() EntrySpec { return c.entry }

// LoginUser returns the SSH login user used when the connection was dialed.
func (c *Connection) LoginUser() string { return c.loginUser }

// Client returns the underlying *ssh.Client. Channels use this to spawn new
// SSH sessions of the appropriate type (PTY exec, SFTP subsystem, direct-tcpip).
func (c *Connection) Client() *ssh.Client { return c.client }

// Get returns the channel registered for key, or nil/false if absent.
func (c *Connection) Get(key ChannelKey) (Channel, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch, ok := c.channels[key]
	return ch, ok
}

// Put registers a channel under its key. Existing channels with the same key
// are closed and replaced.
func (c *Connection) Put(ch Channel) {
	if ch == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	newKey := ch.Key()
	for key, existing := range c.channels {
		if existing == ch && key != newKey {
			delete(c.channels, key)
		}
	}
	if existing, ok := c.channels[newKey]; ok && existing != ch {
		_ = existing.Close()
	}
	c.channels[newKey] = ch
}

// Drop closes and removes the channel for key.
func (c *Connection) Drop(key ChannelKey) {
	c.mu.Lock()
	ch, ok := c.channels[key]
	if ok {
		delete(c.channels, key)
	}
	c.mu.Unlock()
	if ok && ch != nil {
		_ = ch.Close()
	}
}

// Snapshots returns a copy of every channel snapshot for this connection.
func (c *Connection) Snapshots() []ChannelSnapshot {
	c.mu.Lock()
	out := make([]ChannelSnapshot, 0, len(c.channels))
	for _, ch := range c.channels {
		out = append(out, ch.Snapshot())
	}
	c.mu.Unlock()
	return out
}

// Keepalive sends an OpenSSH keepalive request. Returns an error if the
// underlying SSH transport is dead so the pool can mark the connection as
// closed and force a redial.
func (c *Connection) Keepalive() error {
	if c.closed.Load() {
		return fmt.Errorf("connection %s is closed", c.alias)
	}
	if c.client == nil {
		return fmt.Errorf("connection %s has no client", c.alias)
	}
	_, _, err := c.client.SendRequest("keepalive@openssh.com", true, nil)
	return err
}

// IsClosed reports whether Close was called.
func (c *Connection) IsClosed() bool { return c.closed.Load() }

// Close shuts down every channel and the underlying SSH client. Safe to call
// multiple times.
func (c *Connection) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	c.mu.Lock()
	channels := make([]Channel, 0, len(c.channels))
	for _, ch := range c.channels {
		channels = append(channels, ch)
	}
	c.channels = map[ChannelKey]Channel{}
	c.mu.Unlock()

	for _, ch := range channels {
		_ = ch.Close()
	}
	var closeErr error
	if c.client != nil {
		closeErr = c.client.Close()
	}
	closeAll(c.authClosers)
	return closeErr
}
