package lane

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"golang.org/x/crypto/ssh"
)

// ErrNotImplemented is returned for operations that have not yet been wired in.
var ErrNotImplemented = errors.New("lane: not implemented")

// ErrUnknownLane is returned when an operation references a lane id the
// manager has no record of.
var ErrUnknownLane = errors.New("lane: unknown id")

// SSHClientFactory resolves an alias to a connected *ssh.Client. The lane
// manager owns no SSH connections of its own; the caller (workspace.Manager)
// retains responsibility for the connection pool.
type SSHClientFactory func(ctx context.Context, alias string) (client *ssh.Client, loginUser, defaultCWD string, err error)

// NewNoopManager returns a LaneManager that rejects every Acquire with
// ErrNotImplemented. Useful for unit tests and as a placeholder before the
// SSH-backed manager is wired in.
func NewNoopManager() LaneManager { return &noopManager{} }

type noopManager struct{}

func (n *noopManager) Acquire(ctx context.Context, t Target, opts AcquireOpts) (ExecutionLane, error) {
	return nil, ErrNotImplemented
}

func (n *noopManager) Release(id LaneID) {}

func (n *noopManager) Snapshots() []LaneSnapshot { return nil }

func (n *noopManager) CloseAll() error { return nil }

// NewManager returns a LaneManager that opens persistent PTY shells via the
// SSH clients provided by factory. Lanes are cached by alias; account stack
// changes (su / sudo -i / exit) are tracked inside each lane, not as separate
// cache entries.
func NewManager(factory SSHClientFactory) LaneManager {
	return &manager{
		factory: factory,
		lanes:   make(map[string]*sshLane),
	}
}

type manager struct {
	factory SSHClientFactory
	mu      sync.Mutex
	lanes   map[string]*sshLane
}

func (m *manager) Acquire(ctx context.Context, t Target, opts AcquireOpts) (ExecutionLane, error) {
	if m.factory == nil {
		return nil, errors.New("lane: ssh client factory not configured")
	}
	if t.Alias == "" {
		return nil, errors.New("lane: target alias is required")
	}

	m.mu.Lock()
	if existing, ok := m.lanes[t.Alias]; ok && !existing.closed.Load() {
		m.mu.Unlock()
		return existing, nil
	}
	m.mu.Unlock()

	client, loginUser, defaultCWD, err := m.factory(ctx, t.Alias)
	if err != nil {
		return nil, fmt.Errorf("lane: resolve ssh client for %s: %w", t.Alias, err)
	}
	if client == nil {
		return nil, fmt.Errorf("lane: ssh client for %s is nil", t.Alias)
	}

	lane, err := openSSHLane(ctx, client, t.Alias, loginUser, defaultCWD)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	if existing, ok := m.lanes[t.Alias]; ok && !existing.closed.Load() {
		m.mu.Unlock()
		_ = lane.Close()
		return existing, nil
	}
	m.lanes[t.Alias] = lane
	m.mu.Unlock()
	return lane, nil
}

func (m *manager) Release(id LaneID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for alias, l := range m.lanes {
		if l != nil && l.ID() == id {
			delete(m.lanes, alias)
			_ = l.Close()
			return
		}
	}
}

func (m *manager) Snapshots() []LaneSnapshot {
	m.mu.Lock()
	lanes := make([]*sshLane, 0, len(m.lanes))
	for _, l := range m.lanes {
		if l != nil && !l.closed.Load() {
			lanes = append(lanes, l)
		}
	}
	m.mu.Unlock()

	out := make([]LaneSnapshot, 0, len(lanes))
	for _, l := range lanes {
		out = append(out, l.Snapshot())
	}
	return out
}

func (m *manager) CloseAll() error {
	m.mu.Lock()
	lanes := make([]*sshLane, 0, len(m.lanes))
	for _, l := range m.lanes {
		if l != nil {
			lanes = append(lanes, l)
		}
	}
	m.lanes = make(map[string]*sshLane)
	m.mu.Unlock()

	for _, l := range lanes {
		_ = l.Close()
	}
	return nil
}
