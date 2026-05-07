package channel

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

// inventoryChannel runs an arbitrary batched bash script in a fresh SSH
// session on demand. It does NOT touch the exec PTY lane, so concurrent
// user commands are not serialized behind inventory scans.
type inventoryChannel struct {
	key    ChannelKey
	client *ssh.Client

	mu       sync.Mutex
	state    ChannelState
	lastUsed time.Time
	closed   atomic.Bool
}

func openInventoryChannel(client *ssh.Client, alias string) (*inventoryChannel, error) {
	if client == nil {
		return nil, fmt.Errorf("channel: inventory requires a live ssh client")
	}
	return &inventoryChannel{
		key: ChannelKey{
			Alias:     alias,
			Privilege: PrivilegeDefault,
			Purpose:   PurposeInventory,
		},
		client:   client,
		state:    StateReady,
		lastUsed: time.Now(),
	}, nil
}

func (c *inventoryChannel) Key() ChannelKey { return c.key }

func (c *inventoryChannel) Snapshot() ChannelSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return ChannelSnapshot{
		Key:      c.key,
		State:    c.state,
		LastUsed: c.lastUsed,
	}
}

// Run executes script in a fresh ssh.Session and returns stdout. It does NOT
// touch the caller's exec channel, so concurrent user commands are not
// serialized behind this call.
func (c *inventoryChannel) Run(ctx context.Context, script string) (string, error) {
	if c.closed.Load() {
		return "", fmt.Errorf("channel: inventory closed")
	}

	c.mu.Lock()
	c.state = StateBusy
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.state = StateReady
		c.lastUsed = time.Now()
		c.mu.Unlock()
	}()

	session, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("inventory: open session: %w", err)
	}
	defer session.Close()

	var out bytes.Buffer
	session.Stdout = &out

	done := make(chan error, 1)
	go func() {
		done <- session.Run(script)
	}()

	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGTERM)
		<-done
		return "", ctx.Err()
	case <-done:
		return out.String(), nil
	}
}

func (c *inventoryChannel) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	c.mu.Lock()
	c.state = StateClosed
	c.mu.Unlock()
	return nil
}
