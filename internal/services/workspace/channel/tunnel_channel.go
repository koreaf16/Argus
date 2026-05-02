package channel

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

// tunnelChannel registers SSH port-forward listeners for one host. It is
// pinned to PrivilegeDefault because port forwarding rides directly on the
// master *ssh.Client.
type tunnelChannel struct {
	key    ChannelKey
	alias  string
	client *ssh.Client

	mu       sync.RWMutex
	tunnels  map[string]*sshTunnel
	tunnelSeq atomic.Uint64
	state    ChannelState
	lastUsed time.Time
	closed   atomic.Bool
}

type sshTunnel struct {
	id         string
	localAddr  string
	remoteAddr string
	listener   net.Listener
	closed     atomic.Bool
	done       chan struct{}
	closeOnce  sync.Once
}

func (t *sshTunnel) close() error {
	var err error
	t.closeOnce.Do(func() {
		t.closed.Store(true)
		err = t.listener.Close()
	})
	return err
}

func openTunnelChannel(client *ssh.Client, alias string) (*tunnelChannel, error) {
	if client == nil {
		return nil, fmt.Errorf("channel: tunnel requires a live ssh client")
	}
	return &tunnelChannel{
		key: ChannelKey{
			Alias:     alias,
			Privilege: PrivilegeDefault,
			Purpose:   PurposeTunnel,
		},
		alias:    alias,
		client:   client,
		tunnels:  make(map[string]*sshTunnel),
		state:    StateReady,
		lastUsed: time.Now(),
	}, nil
}

func (c *tunnelChannel) Key() ChannelKey { return c.key }

func (c *tunnelChannel) Snapshot() ChannelSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return ChannelSnapshot{
		Key:      c.key,
		State:    c.state,
		LastUsed: c.lastUsed,
	}
}

func (c *tunnelChannel) Open(localAddr, remoteAddr string) (TunnelInfo, error) {
	if c.closed.Load() {
		return TunnelInfo{}, errors.New("channel: tunnel closed")
	}
	if strings.TrimSpace(localAddr) == "" {
		localAddr = "127.0.0.1:0"
	}
	if strings.TrimSpace(remoteAddr) == "" {
		return TunnelInfo{}, fmt.Errorf("remote address is required")
	}

	listener, err := net.Listen("tcp", localAddr)
	if err != nil {
		return TunnelInfo{}, err
	}

	id := strconv.FormatUint(c.tunnelSeq.Add(1), 36)
	tunnel := &sshTunnel{
		id:         id,
		localAddr:  listener.Addr().String(),
		remoteAddr: remoteAddr,
		listener:   listener,
		done:       make(chan struct{}),
	}

	c.mu.Lock()
	c.tunnels[id] = tunnel
	c.lastUsed = time.Now()
	c.mu.Unlock()

	go c.run(tunnel)

	return TunnelInfo{
		ID:         id,
		Alias:      c.alias,
		LocalAddr:  tunnel.localAddr,
		RemoteAddr: tunnel.remoteAddr,
		Active:     true,
	}, nil
}

func (c *tunnelChannel) run(tunnel *sshTunnel) {
	defer close(tunnel.done)
	for {
		conn, err := tunnel.listener.Accept()
		if err != nil {
			if tunnel.closed.Load() {
				return
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Temporary() {
				continue
			}
			return
		}
		go c.proxy(conn, tunnel.remoteAddr)
	}
}

func (c *tunnelChannel) proxy(localConn net.Conn, remoteAddr string) {
	remoteConn, err := c.client.Dial("tcp", remoteAddr)
	if err != nil {
		_ = localConn.Close()
		return
	}
	go func() {
		defer func() { _ = recover() }()
		_, _ = io.Copy(remoteConn, localConn)
		_ = remoteConn.Close()
	}()
	go func() {
		defer func() { _ = recover() }()
		_, _ = io.Copy(localConn, remoteConn)
		_ = localConn.Close()
	}()
}

func (c *tunnelChannel) CloseTunnel(id string) error {
	c.mu.Lock()
	tunnel, ok := c.tunnels[id]
	if ok {
		delete(c.tunnels, id)
	}
	c.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown tunnel id: %s", id)
	}
	return tunnel.close()
}

func (c *tunnelChannel) List() []TunnelInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]TunnelInfo, 0, len(c.tunnels))
	for _, t := range c.tunnels {
		out = append(out, TunnelInfo{
			ID:         t.id,
			Alias:      c.alias,
			LocalAddr:  t.localAddr,
			RemoteAddr: t.remoteAddr,
			Active:     !t.closed.Load(),
		})
	}
	return out
}

func (c *tunnelChannel) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	c.mu.Lock()
	tunnels := make([]*sshTunnel, 0, len(c.tunnels))
	for _, t := range c.tunnels {
		tunnels = append(tunnels, t)
	}
	c.tunnels = map[string]*sshTunnel{}
	c.state = StateClosed
	c.mu.Unlock()

	for _, t := range tunnels {
		_ = t.close()
		select {
		case <-t.done:
		case <-time.After(500 * time.Millisecond):
		}
	}
	return nil
}
