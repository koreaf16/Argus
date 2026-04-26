// Package cache
// File: lru.go
// Description: 제네릭 LRU 캐시 (크기 기반 eviction + TTL 자동 만료)
// Responsibility: 크기와 항목 수를 동시에 제한하는 스레드 안전 LRU 캐시 제공

package cache

import (
	"container/list"
	"log/slog"
	"sync"
	"time"
)

// Config는 Cache 생성 설정이다.
type Config struct {
	MaxEntries int           // 최대 항목 수 (0이면 제한 없음)
	MaxBytes   int64         // 최대 크기 바이트 (0이면 제한 없음)
	TTL        time.Duration // 항목 유효 시간 (0이면 만료 없음)
	Name       string        // slog 식별자
}

type entry[K comparable, V any] struct {
	key       K
	val       V
	size      int64
	expiresAt time.Time
	elem      *list.Element
}

// Cache는 크기 기반 eviction과 TTL을 지원하는 제네릭 LRU 캐시이다.
// 모든 공개 메서드는 goroutine-safe하다.
type Cache[K comparable, V any] struct {
	mu        sync.Mutex
	items     map[K]*entry[K, V]
	order     *list.List
	cfg       Config
	usedBytes int64
	stats     Stats
	sizeFn    func(V) int64
}

// NewCache는 sizeFn으로 값 크기를 계산하는 Cache를 생성한다.
// sizeFn이 nil이면 모든 값의 크기를 1로 간주한다.
func NewCache[K comparable, V any](cfg Config, sizeFn func(V) int64) *Cache[K, V] {
	if sizeFn == nil {
		sizeFn = func(V) int64 { return 1 }
	}
	return &Cache[K, V]{
		items:  make(map[K]*entry[K, V]),
		order:  list.New(),
		cfg:    cfg,
		sizeFn: sizeFn,
	}
}

// Get은 키에 대한 캐시 값을 반환한다.
// 만료된 항목은 제거하고 false를 반환한다.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.items[key]
	if !ok {
		c.stats.Misses++
		var zero V
		return zero, false
	}

	if c.isExpired(e) {
		c.removeEntry(e)
		c.stats.Misses++
		c.stats.ExpiredEvictions++
		var zero V
		return zero, false
	}

	c.order.MoveToFront(e.elem)
	c.stats.Hits++
	return e.val, true
}

// Set은 키-값 쌍을 캐시에 저장한다.
// 이미 존재하면 갱신하고, 용량 초과 시 LRU 항목을 제거한다.
func (c *Cache[K, V]) Set(key K, val V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	size := c.sizeFn(val)

	// 기존 항목 갱신
	if e, ok := c.items[key]; ok {
		c.usedBytes -= e.size
		e.val = val
		e.size = size
		e.expiresAt = c.expireTime()
		c.usedBytes += size
		c.order.MoveToFront(e.elem)
		return
	}

	// 용량 확보: 만료 항목 먼저, 그 다음 LRU 순서
	c.evictExpired()
	for c.overCapacity(size) {
		if !c.evictLRU() {
			break
		}
	}

	e := &entry[K, V]{
		key:       key,
		val:       val,
		size:      size,
		expiresAt: c.expireTime(),
	}
	e.elem = c.order.PushFront(e)
	c.items[key] = e
	c.usedBytes += size
}

// Delete는 키에 해당하는 항목을 삭제한다.
func (c *Cache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if e, ok := c.items[key]; ok {
		c.removeEntry(e)
	}
}

// Len은 현재 캐시에 저장된 항목 수를 반환한다.
func (c *Cache[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// BytesUsed는 현재 캐시가 사용하는 총 바이트 수를 반환한다.
func (c *Cache[K, V]) BytesUsed() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.usedBytes
}

// Stats는 현재 캐시 통계를 반환한다.
func (c *Cache[K, V]) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stats
}

// LogStats는 캐시 통계를 slog.Info로 출력한다.
func (c *Cache[K, V]) LogStats() {
	c.mu.Lock()
	s := c.stats
	name := c.cfg.Name
	used := c.usedBytes
	count := len(c.items)
	c.mu.Unlock()

	slog.Info("cache stats",
		"name", name,
		"hits", s.Hits,
		"misses", s.Misses,
		"evictions", s.Evictions,
		"expired_evictions", s.ExpiredEvictions,
		"items", count,
		"bytes_used", used,
	)
}

// --- 내부 헬퍼 (mu 잠긴 상태에서 호출) ---

func (c *Cache[K, V]) isExpired(e *entry[K, V]) bool {
	if c.cfg.TTL <= 0 {
		return false
	}
	return time.Now().After(e.expiresAt)
}

func (c *Cache[K, V]) expireTime() time.Time {
	if c.cfg.TTL <= 0 {
		return time.Time{}
	}
	return time.Now().Add(c.cfg.TTL)
}

func (c *Cache[K, V]) overCapacity(newSize int64) bool {
	if c.cfg.MaxEntries > 0 && len(c.items) >= c.cfg.MaxEntries {
		return true
	}
	if c.cfg.MaxBytes > 0 && c.usedBytes+newSize > c.cfg.MaxBytes {
		return true
	}
	return false
}

func (c *Cache[K, V]) evictExpired() {
	now := time.Now()
	if c.cfg.TTL <= 0 {
		return
	}
	for _, e := range c.items {
		if now.After(e.expiresAt) {
			c.removeEntry(e)
			c.stats.ExpiredEvictions++
		}
	}
}

func (c *Cache[K, V]) evictLRU() bool {
	oldest := c.order.Back()
	if oldest == nil {
		return false
	}
	e := oldest.Value.(*entry[K, V])
	c.removeEntry(e)
	c.stats.Evictions++
	return true
}

func (c *Cache[K, V]) removeEntry(e *entry[K, V]) {
	c.order.Remove(e.elem)
	delete(c.items, e.key)
	c.usedBytes -= e.size
}
