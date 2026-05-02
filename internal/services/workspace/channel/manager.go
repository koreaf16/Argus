package channel

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/koreaf16/argus/internal/services/workspace/lane"
)

// ChannelManager owns the lifecycle of every virtual channel across all hosts.
// It uses ConnectionPool to dial master SSH connections lazily and creates
// purpose-specific channels on first acquire. Privilege-scoped exec channels
// are spawned by issuing the appropriate enter command on a default exec
// channel, then re-keying the resulting child shell.
type ChannelManager interface {
	// AcquireExec returns the exec channel for (alias, priv). When priv is
	// PrivilegeDefault and no channel exists yet, a fresh PTY is opened.
	// For non-default priv, the manager refuses to spawn the channel itself —
	// callers must route through the default exec channel's EnterAccount and
	// register the resulting privilege channel via PromoteExec.
	AcquireExec(ctx context.Context, alias string, priv PrivilegeKey, opts AcquireOpts) (ExecCapable, error)

	// PromoteExec registers a privilege-scoped channel for an alias. It is
	// invoked after a successful EnterAccount on the default channel: the
	// default channel itself has migrated to the new privilege key, so this
	// call moves the cached entry under the new key and re-opens a fresh
	// default channel for subsequent default-privilege traffic.
	PromoteExec(ctx context.Context, alias string, child ExecCapable) error

	AcquireMetrics(ctx context.Context, alias string) (MetricsCapable, error)
	AcquireSFTP(ctx context.Context, alias string) (SFTPCapable, error)
	AcquireTunnel(ctx context.Context, alias string) (TunnelCapable, error)

	Snapshots() []ChannelSnapshot
	DropChannel(key ChannelKey)
	DropAlias(alias string) error
	CloseAll() error
}

type managerImpl struct {
	pool *ConnectionPool
	mu   sync.Mutex
}

// NewChannelManager builds a manager backed by pool.
func NewChannelManager(pool *ConnectionPool) ChannelManager {
	if pool == nil {
		panic("channel: NewChannelManager requires a non-nil ConnectionPool")
	}
	return &managerImpl{pool: pool}
}

// AcquireExec returns the exec channel for (alias, priv). The manager only
// auto-spawns PrivilegeDefault channels; non-default privileges must be
// promoted by the caller after an EnterAccount on the default channel.
func (m *managerImpl) AcquireExec(ctx context.Context, alias string, priv PrivilegeKey, opts AcquireOpts) (ExecCapable, error) {
	conn, err := m.pool.Get(ctx, alias)
	if err != nil {
		return nil, err
	}
	key := ChannelKey{Alias: alias, Privilege: priv, Purpose: PurposeExec}

	if existing, ok := conn.Get(key); ok {
		if ec, ok := existing.(ExecCapable); ok {
			return ec, nil
		}
		conn.Drop(key)
	}

	if priv != PrivilegeDefault {
		return nil, fmt.Errorf("channel: privilege %q not active on %s; enter via the default channel first", priv, alias)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// Re-check after acquiring the manager lock to avoid double-spawning when
	// concurrent acquires race for the same alias.
	if existing, ok := conn.Get(key); ok {
		if ec, ok := existing.(ExecCapable); ok {
			return ec, nil
		}
		conn.Drop(key)
	}

	ch, err := openExecChannel(ctx, conn.Client(), alias, conn.LoginUser(), conn.Entry().DefaultCWD)
	if err != nil {
		return nil, err
	}
	conn.Put(ch)
	return ch, nil
}

// PromoteExec re-registers a child exec channel under its post-EnterAccount
// privilege key and opens a fresh default exec channel for subsequent
// default-privilege traffic. The previous default channel is moved to the new
// privilege slot in the registry.
//
// In the current single-PTY-per-privilege model this is a no-op for the
// caller: the same execChannel changes its key in place during EnterAccount,
// and the next AcquireExec(default) will spawn a brand-new shell. We expose
// the method so the public API stays stable as the implementation evolves.
func (m *managerImpl) PromoteExec(ctx context.Context, alias string, child ExecCapable) error {
	if child == nil {
		return errors.New("channel: nil child")
	}
	conn, err := m.pool.Get(ctx, alias)
	if err != nil {
		return err
	}
	conn.Put(child)
	return nil
}

func (m *managerImpl) AcquireMetrics(ctx context.Context, alias string) (MetricsCapable, error) {
	conn, err := m.pool.Get(ctx, alias)
	if err != nil {
		return nil, err
	}
	key := ChannelKey{Alias: alias, Privilege: PrivilegeDefault, Purpose: PurposeMetrics}

	if existing, ok := conn.Get(key); ok {
		if mc, ok := existing.(MetricsCapable); ok {
			return mc, nil
		}
		conn.Drop(key)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := conn.Get(key); ok {
		if mc, ok := existing.(MetricsCapable); ok {
			return mc, nil
		}
		conn.Drop(key)
	}

	ch, err := openMetricsChannel(conn.Client(), alias)
	if err != nil {
		return nil, err
	}
	conn.Put(ch)
	return ch, nil
}

func (m *managerImpl) AcquireSFTP(ctx context.Context, alias string) (SFTPCapable, error) {
	conn, err := m.pool.Get(ctx, alias)
	if err != nil {
		return nil, err
	}
	key := ChannelKey{Alias: alias, Privilege: PrivilegeDefault, Purpose: PurposeSFTP}

	if existing, ok := conn.Get(key); ok {
		if sc, ok := existing.(SFTPCapable); ok {
			return sc, nil
		}
		conn.Drop(key)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := conn.Get(key); ok {
		if sc, ok := existing.(SFTPCapable); ok {
			return sc, nil
		}
		conn.Drop(key)
	}

	ch, err := openSFTPChannel(conn.Client(), alias)
	if err != nil {
		return nil, err
	}
	conn.Put(ch)
	return ch, nil
}

func (m *managerImpl) AcquireTunnel(ctx context.Context, alias string) (TunnelCapable, error) {
	conn, err := m.pool.Get(ctx, alias)
	if err != nil {
		return nil, err
	}
	key := ChannelKey{Alias: alias, Privilege: PrivilegeDefault, Purpose: PurposeTunnel}

	if existing, ok := conn.Get(key); ok {
		if tc, ok := existing.(TunnelCapable); ok {
			return tc, nil
		}
		conn.Drop(key)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := conn.Get(key); ok {
		if tc, ok := existing.(TunnelCapable); ok {
			return tc, nil
		}
		conn.Drop(key)
	}

	ch, err := openTunnelChannel(conn.Client(), alias)
	if err != nil {
		return nil, err
	}
	conn.Put(ch)
	return ch, nil
}

func (m *managerImpl) Snapshots() []ChannelSnapshot {
	out := []ChannelSnapshot{}
	for _, alias := range m.pool.Aliases() {
		conn := m.pool.Lookup(alias)
		if conn == nil {
			continue
		}
		out = append(out, conn.Snapshots()...)
	}
	return out
}

func (m *managerImpl) DropChannel(key ChannelKey) {
	conn := m.pool.Lookup(key.Alias)
	if conn == nil {
		return
	}
	conn.Drop(key)
}

func (m *managerImpl) DropAlias(alias string) error {
	return m.pool.Drop(alias)
}

func (m *managerImpl) CloseAll() error {
	return m.pool.DropAll()
}

// LookupExecCurrentStack walks the cached channels for alias and returns the
// account stack of the most recently used exec channel, plus the connection's
// login user. Used by the workspace.Manager to feed RouteFor without forcing
// a default-channel acquire on every routing decision.
func LookupExecCurrentStack(pool *ConnectionPool, alias string) (loginUser string, stack lane.AccountStack, ok bool) {
	conn := pool.Lookup(alias)
	if conn == nil {
		return "", nil, false
	}
	loginUser = conn.LoginUser()
	var newest *ChannelSnapshot
	for _, snap := range conn.Snapshots() {
		if snap.Key.Purpose != PurposeExec {
			continue
		}
		if newest == nil || snap.LastUsed.After(newest.LastUsed) {
			s := snap
			newest = &s
		}
	}
	if newest == nil {
		return loginUser, lane.AccountStack{}, false
	}
	return loginUser, newest.AccountStack.Clone(), true
}
