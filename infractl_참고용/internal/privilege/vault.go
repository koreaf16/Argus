// Package privilege
// File: vault.go
// Description: 세션 스코프 크리덴셜 저장소 — 기존 Cache에 TTL·세션 추적을 추가하는 facade
// Responsibility: Register/Lookup/PurgeSession/WipeAll/List — 비밀번호를 로그에 절대 노출하지 않음

package privilege

import (
	"errors"
	"log/slog"
	"sync"
	"time"
)

const defaultTTL = 30 * time.Minute

// Scope defines the lifetime scope of a vault entry.
type Scope string

const (
	// ScopeSession entries live only until the session ends or the TTL expires.
	ScopeSession Scope = "session"
)

// VaultEntry holds metadata about a stored credential (never the password itself).
type VaultEntry struct {
	Target  string
	Method  Method
	User    string
	Scope   Scope
	TTL     time.Duration
	AddedAt time.Time
}

// Vault wraps Cache with per-session tracking and TTL-based expiry.
// All methods are safe for concurrent use.
type Vault struct {
	mu       sync.RWMutex
	core     *Cache
	entries  map[CacheKey]VaultEntry
	sessions map[string]map[CacheKey]struct{} // sessionID → set of cache keys
	clock    func() time.Time
}

// NewVault creates a Vault backed by the provided Cache.
func NewVault(core *Cache) *Vault {
	return &Vault{
		core:     core,
		entries:  make(map[CacheKey]VaultEntry),
		sessions: make(map[string]map[CacheKey]struct{}),
		clock:    time.Now,
	}
}

// Register stores a credential in the vault under the given session scope.
// sessionID must not be empty. ttl=0 defaults to 30 minutes.
// The password value is never written to any log.
func (v *Vault) Register(sessionID, target string, m Method, user, password string, ttl time.Duration) error {
	if sessionID == "" {
		return errors.New("session id required")
	}
	if ttl <= 0 {
		ttl = defaultTTL
	}
	key := cacheKey(target, m, user)
	entry := VaultEntry{
		Target:  key.Target,
		Method:  m,
		User:    key.User,
		Scope:   ScopeSession,
		TTL:     ttl,
		AddedAt: v.clock(),
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	v.core.Set(target, m, user, password)
	v.entries[key] = entry
	if v.sessions[sessionID] == nil {
		v.sessions[sessionID] = make(map[CacheKey]struct{})
	}
	v.sessions[sessionID][key] = struct{}{}

	// Log metadata only — password is never included.
	slog.Debug("vault register", "target", key.Target, "user", key.User, "method", m, "ttl", ttl)
	return nil
}

// Lookup retrieves the password for the given target/method/user combination.
// Returns ("", false) when the entry does not exist or has expired.
// Expired entries are deleted automatically.
func (v *Vault) Lookup(target string, m Method, user string) (string, bool) {
	key := cacheKey(target, m, user)

	v.mu.Lock()
	defer v.mu.Unlock()

	entry, ok := v.entries[key]
	if !ok {
		return "", false
	}
	if v.clock().After(entry.AddedAt.Add(entry.TTL)) {
		// expired — clean up silently
		delete(v.entries, key)
		v.core.Delete(entry.Target, entry.Method, entry.User)
		return "", false
	}
	pw, found := v.core.Get(target, m, user)
	return pw, found
}

// PurgeSession removes all credentials registered under the given session.
func (v *Vault) PurgeSession(sessionID string) {
	v.mu.Lock()
	defer v.mu.Unlock()

	keys, ok := v.sessions[sessionID]
	if !ok {
		return
	}
	for key := range keys {
		delete(v.entries, key)
		v.core.Delete(key.Target, key.Method, key.User)
	}
	delete(v.sessions, sessionID)
	slog.Debug("vault purge session", "session", sessionID)
}

// WipeAll removes every credential from the vault and the underlying cache.
// Intended to be called on process shutdown.
func (v *Vault) WipeAll() {
	v.mu.Lock()
	defer v.mu.Unlock()

	for key, entry := range v.entries {
		v.core.Delete(entry.Target, entry.Method, entry.User)
		delete(v.entries, key)
	}
	v.sessions = make(map[string]map[CacheKey]struct{})
	slog.Debug("vault wiped all entries")
}

// List returns a snapshot of all live entry metadata.
// Passwords are never included in the returned slice.
func (v *Vault) List() []VaultEntry {
	v.mu.RLock()
	defer v.mu.RUnlock()

	out := make([]VaultEntry, 0, len(v.entries))
	for _, e := range v.entries {
		out = append(out, e)
	}
	return out
}
