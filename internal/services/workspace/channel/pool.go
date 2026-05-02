package channel

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// CredentialClearer is implemented by the password cache so the pool can clear
// stale credentials after an AuthFailedError before re-prompting.
type CredentialClearer interface {
	ClearAlias(alias string)
}

// EntryResolver looks up server entries by alias. The pool injects this so it
// does not depend on workspace.Registry directly (testability). The returned
// EntrySpec must describe an SSH-kind server; non-SSH callers are rejected
// before reaching ConnectionPool.
type EntryResolver interface {
	ResolveEntry(alias string) (EntrySpec, error)
}

// ConnectionPool owns one master *ssh.Client per host alias. Channels are
// multiplexed on top of these clients via Connection.
type ConnectionPool struct {
	mu          sync.Mutex
	connections map[string]*Connection
	prompt      PasswordPrompt
	resolver    EntryResolver
	clearer     CredentialClearer
}

// NewConnectionPool builds a pool. resolver and prompt must be non-nil.
// clearer may be nil; if provided, the pool will invalidate credentials after
// an AuthFailedError before retrying with a fresh prompt.
func NewConnectionPool(resolver EntryResolver, prompt PasswordPrompt, clearer CredentialClearer) *ConnectionPool {
	return &ConnectionPool{
		connections: make(map[string]*Connection),
		prompt:      prompt,
		resolver:    resolver,
		clearer:     clearer,
	}
}

// Get returns a live Connection for alias. If a cached connection exists, the
// pool keepalives it and returns it on success; on failure, it is closed and
// replaced. Authentication failures retry once with cleared credentials.
func (p *ConnectionPool) Get(ctx context.Context, alias string) (*Connection, error) {
	p.mu.Lock()
	if existing, ok := p.connections[alias]; ok && !existing.IsClosed() {
		if err := existing.Keepalive(); err == nil {
			p.mu.Unlock()
			return existing, nil
		}
		// Stale; tear it down before redialing.
		delete(p.connections, alias)
		p.mu.Unlock()
		_ = existing.Close()
		p.mu.Lock()
	}
	p.mu.Unlock()

	if p.resolver == nil {
		return nil, fmt.Errorf("connection pool has no resolver")
	}
	entry, err := p.resolver.ResolveEntry(alias)
	if err != nil {
		return nil, err
	}

	conn, err := p.dialWithAuthRetry(ctx, entry)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	if existing, ok := p.connections[alias]; ok && !existing.IsClosed() {
		// Race: another goroutine dialed first. Drop ours and use theirs.
		p.mu.Unlock()
		_ = conn.Close()
		return existing, nil
	}
	p.connections[alias] = conn
	p.mu.Unlock()
	return conn, nil
}

func (p *ConnectionPool) dialWithAuthRetry(ctx context.Context, entry EntrySpec) (*Connection, error) {
	conn, err := dialConnection(ctx, entry, p.prompt)
	if err == nil {
		return conn, nil
	}
	var afe *AuthFailedError
	if !errors.As(err, &afe) {
		return nil, err
	}
	// Auth failure: clear cached credentials and retry once with a fresh prompt.
	if p.clearer != nil {
		p.clearer.ClearAlias(entry.Alias)
	}
	conn, err = dialConnection(ctx, entry, p.prompt)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// Drop closes and removes the connection for alias. Channels owned by the
// connection are closed first.
func (p *ConnectionPool) Drop(alias string) error {
	p.mu.Lock()
	conn, ok := p.connections[alias]
	if ok {
		delete(p.connections, alias)
	}
	p.mu.Unlock()
	if !ok || conn == nil {
		return nil
	}
	return conn.Close()
}

// DropAll closes every connection in the pool.
func (p *ConnectionPool) DropAll() error {
	p.mu.Lock()
	conns := make([]*Connection, 0, len(p.connections))
	for _, c := range p.connections {
		conns = append(conns, c)
	}
	p.connections = map[string]*Connection{}
	p.mu.Unlock()

	var firstErr error
	for _, c := range conns {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Aliases returns a snapshot of aliases with live connections.
func (p *ConnectionPool) Aliases() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.connections))
	for a := range p.connections {
		out = append(out, a)
	}
	return out
}

// Lookup returns the cached connection without dialing. Returns nil if no
// connection is cached.
func (p *ConnectionPool) Lookup(alias string) *Connection {
	p.mu.Lock()
	defer p.mu.Unlock()
	conn := p.connections[alias]
	if conn != nil && conn.IsClosed() {
		return nil
	}
	return conn
}

// SetPrompt swaps the password prompt callback. Useful when the TUI replaces
// the prompt function after construction.
func (p *ConnectionPool) SetPrompt(fn PasswordPrompt) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prompt = fn
}
