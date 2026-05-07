package inventory

import (
	"sync"
	"time"
)

const defaultTTL = 5 * time.Minute

// cache is a simple alias → (snapshot, expiry) memory map.
type cache struct {
	mu    sync.RWMutex
	items map[string]cacheEntry
}

type cacheEntry struct {
	snap    InventorySnapshot
	expires time.Time
}

func newCache() *cache {
	return &cache{items: make(map[string]cacheEntry)}
}

// Get returns the snapshot for alias if it exists and has not expired.
func (c *cache) Get(alias string) (InventorySnapshot, bool) {
	c.mu.RLock()
	e, ok := c.items[alias]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.expires) {
		return InventorySnapshot{}, false
	}
	return e.snap, true
}

// Set stores snap with the default TTL.
func (c *cache) Set(alias string, snap InventorySnapshot) {
	c.mu.Lock()
	c.items[alias] = cacheEntry{snap: snap, expires: time.Now().Add(defaultTTL)}
	c.mu.Unlock()
}

// Invalidate removes the entry for alias (memory only).
func (c *cache) Invalidate(alias string) {
	c.mu.Lock()
	delete(c.items, alias)
	c.mu.Unlock()
}
