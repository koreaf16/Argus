package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestCache_BasicGetSet(t *testing.T) {
	c := NewCache[string, string](Config{
		MaxEntries: 10,
		Name:       "test",
	}, StringSizeFn)

	c.Set("k1", "v1")
	c.Set("k2", "v2")

	v, ok := c.Get("k1")
	if !ok || v != "v1" {
		t.Fatalf("Get(k1) = %q, %v; want v1, true", v, ok)
	}

	_, ok = c.Get("missing")
	if ok {
		t.Fatal("Get(missing) should return false")
	}
}

func TestCache_TTLExpiry(t *testing.T) {
	c := NewCache[string, string](Config{
		MaxEntries: 10,
		TTL:        50 * time.Millisecond,
		Name:       "ttl-test",
	}, StringSizeFn)

	c.Set("key", "value")

	v, ok := c.Get("key")
	if !ok || v != "value" {
		t.Fatalf("before expiry: Get = %q, %v; want value, true", v, ok)
	}

	time.Sleep(100 * time.Millisecond)

	_, ok = c.Get("key")
	if ok {
		t.Fatal("after expiry: Get should return false")
	}

	stats := c.Stats()
	if stats.ExpiredEvictions < 1 {
		t.Fatalf("ExpiredEvictions = %d; want >= 1", stats.ExpiredEvictions)
	}
}

func TestCache_LRUEviction(t *testing.T) {
	c := NewCache[string, string](Config{
		MaxEntries: 3,
		Name:       "lru-test",
	}, StringSizeFn)

	c.Set("a", "1")
	c.Set("b", "2")
	c.Set("c", "3")

	c.Get("b")
	c.Set("d", "4")

	_, ok := c.Get("a")
	if ok {
		t.Fatal("a should have been evicted (LRU)")
	}

	_, ok = c.Get("b")
	if !ok {
		t.Fatal("b should still be in cache (was recently used)")
	}
}

func TestCache_SizeBasedEviction(t *testing.T) {
	c := NewCache[string, string](Config{
		MaxBytes: 25,
		Name:     "size-test",
	}, StringSizeFn)

	c.Set("k1", "0123456789")
	c.Set("k2", "0123456789")
	c.Set("k3", "0123456789")

	if c.BytesUsed() > 25 {
		t.Fatalf("BytesUsed = %d; want <= 25", c.BytesUsed())
	}

	_, ok := c.Get("k1")
	if ok {
		t.Fatal("k1 should have been evicted due to size limit")
	}
}

func TestCache_Delete(t *testing.T) {
	c := NewCache[string, string](Config{MaxEntries: 5}, StringSizeFn)

	c.Set("x", "val")
	c.Delete("x")

	_, ok := c.Get("x")
	if ok {
		t.Fatal("deleted key should not exist")
	}
}

func TestCache_UpdateExisting(t *testing.T) {
	c := NewCache[string, string](Config{
		MaxBytes: 100,
		Name:     "update-test",
	}, StringSizeFn)

	c.Set("k", "short")
	c.Set("k", "longervalue")

	v, ok := c.Get("k")
	if !ok || v != "longervalue" {
		t.Fatalf("Get = %q, %v; want longervalue, true", v, ok)
	}

	if c.BytesUsed() != int64(len("longervalue")) {
		t.Fatalf("BytesUsed = %d; want %d", c.BytesUsed(), len("longervalue"))
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	c := NewCache[string, string](Config{
		MaxEntries: 100,
		Name:       "concurrent-test",
	}, StringSizeFn)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		i := i
		go func() {
			defer wg.Done()
			c.Set(fmt.Sprintf("key-%d", i), fmt.Sprintf("val-%d", i))
		}()
		go func() {
			defer wg.Done()
			c.Get(fmt.Sprintf("key-%d", i))
		}()
	}
	wg.Wait()
}

func TestCache_Stats(t *testing.T) {
	c := NewCache[string, string](Config{MaxEntries: 5}, StringSizeFn)

	c.Set("k", "v")
	c.Get("k")
	c.Get("k")
	c.Get("miss1")
	c.Get("miss2")

	s := c.Stats()
	if s.Hits != 2 {
		t.Fatalf("Hits = %d; want 2", s.Hits)
	}
	if s.Misses != 2 {
		t.Fatalf("Misses = %d; want 2", s.Misses)
	}
}
