package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	tool "github.com/koreaf16/argus/internal/tools"
)

// ExecutorFn is an alias for the LLM sub-query function used in intent resolution.
type ExecutorFn = tool.ExecuteSubQueryFunc

// SearchIntent holds LLM-resolved API parameters.
// Each provider reads only the fields relevant to its API.
type SearchIntent struct {
	Keyword     string  `json:"keyword"`
	Sort        string  `json:"sort"`
	Direction   string  `json:"direction"`
	Entity      string  `json:"entity"`
	Filter      string  `json:"filter"`
	Prerelease  string  `json:"prerelease"`
	PackageType string  `json:"package_type"`
	Popularity  float64 `json:"popularity"`
	Quality     float64 `json:"quality"`
	Maintenance float64 `json:"maintenance"`
	Kind        string  `json:"kind"`
}

const intentCacheTTL = 60 * time.Second

var (
	intentCacheMu    sync.Mutex
	intentCacheItems = make(map[string]intentCacheEntry, 64)
)

type intentCacheEntry struct {
	intent   SearchIntent
	cachedAt time.Time
}

// resolveIntent calls the LLM sub-query to extract structured API parameters from a
// natural language query. Returns false if fn is nil, the LLM call fails, or JSON
// cannot be parsed — callers fall back to raw query behavior.
func resolveIntent(ctx context.Context, fn ExecutorFn, providerName, systemPrompt, query string) (SearchIntent, bool) {
	if fn == nil {
		return SearchIntent{}, false
	}
	key := providerName + "\x00" + query

	intentCacheMu.Lock()
	if e, ok := intentCacheItems[key]; ok && time.Since(e.cachedAt) < intentCacheTTL {
		intentCacheMu.Unlock()
		return e.intent, true
	}
	intentCacheMu.Unlock()

	resp, err := fn(ctx, systemPrompt, fmt.Sprintf("Query: %s", query))
	if err != nil {
		return SearchIntent{}, false
	}

	raw := strings.TrimSpace(resp)
	if i := strings.Index(raw, "{"); i > 0 {
		raw = raw[i:]
	}
	if i := strings.LastIndex(raw, "}"); i >= 0 && i < len(raw)-1 {
		raw = raw[:i+1]
	}

	var intent SearchIntent
	if err := json.Unmarshal([]byte(raw), &intent); err != nil {
		return SearchIntent{}, false
	}

	intentCacheMu.Lock()
	intentCacheItems[key] = intentCacheEntry{intent: intent, cachedAt: time.Now()}
	intentCacheMu.Unlock()
	return intent, true
}
