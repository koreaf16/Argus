package connector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Cache struct {
	dir string
}

func NewCache(dir string) *Cache {
	return &Cache{dir: dir}
}

type cacheEntry struct {
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

func (c *Cache) Get(key string, ttl time.Duration) ([]byte, bool) {
	path := filepath.Join(c.dir, key+".json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var entry cacheEntry
	if err := json.Unmarshal(b, &entry); err != nil {
		return nil, false
	}
	if time.Since(entry.Timestamp) > ttl {
		return nil, false
	}
	return entry.Data, true
}

func (c *Cache) Set(key string, data []byte) error {
	if err := os.MkdirAll(c.dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(c.dir, key+".json")
	entry := cacheEntry{
		Timestamp: time.Now(),
		Data:      data,
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}
