package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/web"
)

type scriptedSearchClient struct {
	mu        sync.Mutex
	responses []llm.Response
	idx       int
}

func (c *scriptedSearchClient) Chat(context.Context, []llm.Message, []llm.ToolDef, interface{}, ...llm.CallOption) (llm.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.idx >= len(c.responses) {
		return llm.Response{}, nil
	}
	resp := c.responses[c.idx]
	c.idx++
	return resp, nil
}

func (c *scriptedSearchClient) ChatStream(context.Context, []llm.Message, []llm.ToolDef, interface{}, func(string), func(string), ...llm.CallOption) (llm.Response, error) {
	return llm.Response{}, nil
}

type blockingSearchClient struct {
	active    atomic.Int32
	maxActive atomic.Int32
	release   <-chan struct{}
}

func (c *blockingSearchClient) Chat(ctx context.Context, _ []llm.Message, _ []llm.ToolDef, _ interface{}, _ ...llm.CallOption) (llm.Response, error) {
	now := c.active.Add(1)
	for {
		prev := c.maxActive.Load()
		if now <= prev || c.maxActive.CompareAndSwap(prev, now) {
			break
		}
	}
	defer c.active.Add(-1)
	select {
	case <-ctx.Done():
		return llm.Response{}, ctx.Err()
	case <-c.release:
		return llm.Response{}, nil
	}
}

func (c *blockingSearchClient) ChatStream(context.Context, []llm.Message, []llm.ToolDef, interface{}, func(string), func(string), ...llm.CallOption) (llm.Response, error) {
	return llm.Response{}, nil
}

func searchToolCall(id, q string) llm.Response {
	args, _ := json.Marshal(map[string]string{"query": q})
	return llm.Response{ToolCalls: []llm.ToolCall{{ID: id, Type: "function", Function: llm.FunctionCall{Name: "search", Arguments: string(args)}}}}
}

type searchMeta struct {
	CompletionReason string   `json:"completion_reason"`
	AttemptsUsed     int      `json:"attempts_used"`
	SourcesCount     int      `json:"sources_count"`
	FollowUpRequired bool     `json:"follow_up_required"`
	Queries          []string `json:"queries"`
}

func decodeSearchMeta(t *testing.T, raw string) searchMeta {
	t.Helper()
	var m searchMeta
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	return m
}

func TestInnerLoop_CompletesAfterOneGoodSearch(t *testing.T) {
	tool := &WebSearchTool{
		LLMClient: &scriptedSearchClient{responses: []llm.Response{searchToolCall("c1", "redis latest stable"), {Content: "done"}}},
		RawSearchFn: func(context.Context, string, int, ...web.SearchOption) ([]web.SearchResult, error) {
			return []web.SearchResult{{Title: "Redis Releases", URL: "https://redis.io/docs/latest/operate/oss_and_stack/install/install-redis/", Snippet: "release notes"}}, nil
		},
	}
	out, err := tool.Execute(context.Background(), map[string]interface{}{"query": "redis latest stable"}, webFetchNoopExecutor{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	meta := decodeSearchMeta(t, out.MetadataJSON)
	if meta.CompletionReason != "assistant_complete" || meta.AttemptsUsed != 1 || meta.SourcesCount < 1 || meta.FollowUpRequired {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
}

func TestInnerLoop_NoResults(t *testing.T) {
	tool := &WebSearchTool{
		LLMClient: &scriptedSearchClient{responses: []llm.Response{searchToolCall("c1", "xyz-nope"), {Content: "done"}}},
		RawSearchFn: func(context.Context, string, int, ...web.SearchOption) ([]web.SearchResult, error) {
			return nil, nil
		},
	}
	out, err := tool.Execute(context.Background(), map[string]interface{}{"query": "xyz-nope"}, webFetchNoopExecutor{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	meta := decodeSearchMeta(t, out.MetadataJSON)
	if meta.CompletionReason != "no_results" || !meta.FollowUpRequired || meta.SourcesCount != 0 {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
}

func TestInnerLoop_BudgetExhaustedWithPartial(t *testing.T) {
	responses := make([]llm.Response, 0, maxSearchCalls)
	for i := 0; i < maxSearchCalls; i++ {
		responses = append(responses, searchToolCall(fmt.Sprintf("c%d", i+1), fmt.Sprintf("q%d", i+1)))
	}
	tool := &WebSearchTool{
		LLMClient: &scriptedSearchClient{responses: responses},
		RawSearchFn: func(_ context.Context, q string, _ int, _ ...web.SearchOption) ([]web.SearchResult, error) {
			if q == "q1" {
				return []web.SearchResult{{Title: "A", URL: "https://example.com/a"}}, nil
			}
			return nil, nil
		},
	}
	out, err := tool.Execute(context.Background(), map[string]interface{}{"query": "seed"}, webFetchNoopExecutor{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	meta := decodeSearchMeta(t, out.MetadataJSON)
	if meta.CompletionReason != "budget_exhausted_with_sources" || meta.AttemptsUsed != maxSearchCalls || meta.SourcesCount == 0 {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
}

func TestInnerLoop_BudgetExhaustedNoSources(t *testing.T) {
	responses := make([]llm.Response, 0, maxSearchCalls)
	for i := 0; i < maxSearchCalls; i++ {
		responses = append(responses, searchToolCall(fmt.Sprintf("c%d", i+1), fmt.Sprintf("q%d", i+1)))
	}
	tool := &WebSearchTool{
		LLMClient: &scriptedSearchClient{responses: responses},
		RawSearchFn: func(context.Context, string, int, ...web.SearchOption) ([]web.SearchResult, error) {
			return nil, nil
		},
	}
	out, err := tool.Execute(context.Background(), map[string]interface{}{"query": "seed"}, webFetchNoopExecutor{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	meta := decodeSearchMeta(t, out.MetadataJSON)
	if meta.CompletionReason != "budget_exhausted_no_sources" || meta.AttemptsUsed != maxSearchCalls || !meta.FollowUpRequired {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
}

func TestInnerLoop_LegacyFallbackWhenClientNil(t *testing.T) {
	tool := &WebSearchTool{
		SearchFn: func(context.Context, string, int, ...web.SearchOption) ([]web.SearchResult, error) {
			return []web.SearchResult{{Title: "A", URL: "https://example.com/a"}}, nil
		},
	}
	out, err := tool.Execute(context.Background(), map[string]interface{}{"query": "seed"}, webFetchNoopExecutor{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	meta := decodeSearchMeta(t, out.MetadataJSON)
	if meta.CompletionReason != "legacy_fallback" {
		t.Fatalf("expected legacy_fallback, got %+v", meta)
	}
}

func TestInnerLoop_AdapterPathWhenAllowedDomainKnown(t *testing.T) {
	var adapterCalls, rawCalls int
	tool := &WebSearchTool{
		LLMClient: &scriptedSearchClient{responses: []llm.Response{searchToolCall("c1", "q1"), {Content: "done"}}},
		SearchFn: func(context.Context, string, int, ...web.SearchOption) ([]web.SearchResult, error) {
			adapterCalls++
			return []web.SearchResult{{Title: "A", URL: "https://github.com/org/repo"}}, nil
		},
		RawSearchFn: func(context.Context, string, int, ...web.SearchOption) ([]web.SearchResult, error) {
			rawCalls++
			return nil, nil
		},
	}
	_, err := tool.Execute(context.Background(), map[string]interface{}{"query": "q1", "allowed_domains": []interface{}{"github.com"}}, webFetchNoopExecutor{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if adapterCalls == 0 || rawCalls != 0 {
		t.Fatalf("expected adapter path only, adapter=%d raw=%d", adapterCalls, rawCalls)
	}
}

func TestInnerLoop_DedupsIdenticalQueries(t *testing.T) {
	calls := 0
	tool := &WebSearchTool{
		LLMClient: &scriptedSearchClient{responses: []llm.Response{searchToolCall("c1", "same"), searchToolCall("c2", "same"), {Content: "done"}}},
		RawSearchFn: func(context.Context, string, int, ...web.SearchOption) ([]web.SearchResult, error) {
			calls++
			return []web.SearchResult{{Title: "A", URL: "https://example.com/a"}}, nil
		},
	}
	out, err := tool.Execute(context.Background(), map[string]interface{}{"query": "seed"}, webFetchNoopExecutor{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	meta := decodeSearchMeta(t, out.MetadataJSON)
	if calls != 1 || meta.AttemptsUsed != 1 {
		t.Fatalf("expected deduped query call once, calls=%d metadata=%+v", calls, meta)
	}
}

func TestInnerLoop_RespectsCtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tool := &WebSearchTool{LLMClient: &scriptedSearchClient{responses: []llm.Response{{Content: "done"}}}}
	out, err := tool.Execute(ctx, map[string]interface{}{"query": "seed"}, webFetchNoopExecutor{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if out.Success {
		t.Fatalf("expected canceled execution to fail")
	}
	meta := decodeSearchMeta(t, out.MetadataJSON)
	if meta.CompletionReason != "search_error" {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
}

func TestInnerLoop_ConcurrencyCap(t *testing.T) {
	release := make(chan struct{})
	client := &blockingSearchClient{release: release}
	tool := &WebSearchTool{LLMClient: client}

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = tool.Execute(context.Background(), map[string]interface{}{"query": fmt.Sprintf("q%d", i)}, webFetchNoopExecutor{})
		}(i)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if client.maxActive.Load() >= 4 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(release)
	wg.Wait()

	if got := client.maxActive.Load(); got > 4 {
		t.Fatalf("max concurrent inner loops = %d, want <=4", got)
	}
	if got := client.maxActive.Load(); got < 4 {
		t.Fatalf("expected cap to be exercised, max active=%d", got)
	}
}
