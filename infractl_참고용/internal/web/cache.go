// Package web
// File: cache.go
// Description: HTTP 응답 LRU 캐시 (Deprecated — internal/cache.Cache로 대체됨)
// Responsibility: 하위 호환성 유지를 위해 보존. 신규 코드에서는 cache.NewWebCache() 사용.
//
// Deprecated: 이 파일은 internal/cache 패키지로 대체되었다. fetcher.go 참고.

package web

import (
	"container/list"
	"sync"
	"time"
)

const (
	defaultCacheSize = 50
	defaultCacheTTL  = 15 * time.Minute
)

type cacheEntry struct {
	url      string
	content  string
	expireAt time.Time
	elem     *list.Element
}

// LRUCache는 URL → Markdown 응답을 캐싱하는 LRU 캐시이다.
type LRUCache struct {
	mu      sync.Mutex
	items   map[string]*cacheEntry
	order   *list.List // 앞쪽이 가장 최근 사용
	maxSize int
	ttl     time.Duration
}

// NewLRUCache는 지정된 크기와 TTL로 LRU 캐시를 생성한다.
func NewLRUCache(maxSize int, ttl time.Duration) *LRUCache {
	if maxSize <= 0 {
		maxSize = defaultCacheSize
	}
	if ttl <= 0 {
		ttl = defaultCacheTTL
	}
	return &LRUCache{
		items:   make(map[string]*cacheEntry),
		order:   list.New(),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// Get은 URL에 대한 캐시된 내용을 반환한다. 없거나 만료되면 false를 반환한다.
func (c *LRUCache) Get(url string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.items[url]
	if !ok {
		return "", false
	}
	if time.Now().After(entry.expireAt) {
		// TTL 만료: 항목 제거
		c.order.Remove(entry.elem)
		delete(c.items, url)
		return "", false
	}
	// LRU: 최근 사용으로 이동
	c.order.MoveToFront(entry.elem)
	return entry.content, true
}

// Set은 URL에 대한 내용을 캐시에 저장한다.
func (c *LRUCache) Set(url, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 이미 존재하면 갱신
	if entry, ok := c.items[url]; ok {
		entry.content = content
		entry.expireAt = time.Now().Add(c.ttl)
		c.order.MoveToFront(entry.elem)
		return
	}

	// 용량 초과 시 가장 오래된 항목 제거
	if c.order.Len() >= c.maxSize {
		c.evictOldest()
	}

	entry := &cacheEntry{
		url:      url,
		content:  content,
		expireAt: time.Now().Add(c.ttl),
	}
	entry.elem = c.order.PushFront(entry)
	c.items[url] = entry
}

// evictOldest는 가장 오래된 캐시 항목을 제거한다. mu가 잠긴 상태에서 호출해야 한다.
func (c *LRUCache) evictOldest() {
	oldest := c.order.Back()
	if oldest == nil {
		return
	}
	entry := oldest.Value.(*cacheEntry)
	c.order.Remove(oldest)
	delete(c.items, entry.url)
}
