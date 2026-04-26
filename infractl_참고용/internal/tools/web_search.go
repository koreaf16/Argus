package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/web"
)

const (
	internetCheckHost    = "192.168.0.3:30080"
	internetCheckTTL     = 60 * time.Second
	internetCheckTimeout = 5 * time.Second
	webSearchMaxUses     = 8
	maxSearchCalls       = 8
	maxInnerTurns        = 10
)

var (
	internetCheckMu      sync.Mutex
	internetLastCheckAt  time.Time
	internetLastCheckRes bool

	webSearchInnerLoopSem = make(chan struct{}, 4)
)

// IsInternetAvailable caches SearXNG reachability.
func IsInternetAvailable() bool {
	internetCheckMu.Lock()
	defer internetCheckMu.Unlock()

	if time.Since(internetLastCheckAt) < internetCheckTTL {
		return internetLastCheckRes
	}

	conn, err := net.DialTimeout("tcp", internetCheckHost, internetCheckTimeout)
	if err == nil {
		_ = conn.Close()
		internetLastCheckRes = true
	} else {
		internetLastCheckRes = false
	}
	internetLastCheckAt = time.Now()
	return internetLastCheckRes
}

// WebSearchTool researches external web sources.
type WebSearchTool struct {
	Fetcher      *web.Fetcher
	LLMClient    llm.Client
	LLMRegistry  *llm.Registry
	SearchFn     func(ctx context.Context, query string, limit int, opts ...web.SearchOption) ([]web.SearchResult, error)
	RawSearchFn  func(ctx context.Context, query string, limit int, opts ...web.SearchOption) ([]web.SearchResult, error)
}

func (t *WebSearchTool) Name() string      { return "web_search" }
func (t *WebSearchTool) IsReadOnly() bool  { return true }
func (t *WebSearchTool) IsEnabled() bool   { return true }
func (t *WebSearchTool) IsLocalOnly() bool { return true }

func (t *WebSearchTool) IsConcurrencySafe(_ map[string]interface{}) bool {
	return true
}

func (t *WebSearchTool) Description() string {
	return "Search the web and return source links for current facts, docs, and release notes."
}

func (t *WebSearchTool) ModelPrompt() string {
	currentYear := time.Now().Year()
	return fmt.Sprintf(
		"Use web_search to research external facts. Internally it runs a multi-step search loop, so one call is usually enough. "+
			"If metadata reports completion_reason=no_results or budget_exhausted_no_sources, call web_search again with a different angle, or use web_fetch on the most promising URL. "+
			"Treat queries as seed hints only; the internal loop may reformulate. Do not answer from snippets alone when research metadata says follow-up is required. "+
			"Always include a Sources section with links when web data is used. "+
			"Current year: %d.",
		currentYear,
	)
}

func (t *WebSearchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "A single seed query hint.",
			},
			"queries": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "string",
				},
				"description": "Optional seed query hints for internal reformulation.",
			},
			"allowed_domains": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Only include search results from these domains.",
			},
			"blocked_domains": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Never include search results from these domains.",
			},
		},
	}
}

type queryResult struct {
	query    string
	results  []web.SearchResult
	duration time.Duration
	err      error
}

type webSearchMetadata struct {
	Kind             string   `json:"kind"`
	CompletionReason string   `json:"completion_reason"`
	AttemptsUsed     int      `json:"attempts_used"`
	MaxAttempts      int      `json:"max_attempts"`
	Queries          []string `json:"queries"`
	SourcesCount     int      `json:"sources_count"`
	FollowUpRequired bool     `json:"follow_up_required"`
}

func (t *WebSearchTool) Execute(ctx context.Context, args map[string]interface{}, _ executor.Executor) (ToolOutcome, error) {
	seedQueries := collectSeedQueries(args)
	if len(seedQueries) == 0 {
		msg := "Error: at least one search query is required"
		return ToolOutcome{Content: msg, Success: false, ErrorMessage: msg, MetadataJSON: marshalWebSearchMetadata(webSearchMetadata{
			Kind:             "web_search",
			CompletionReason: "no_results",
			AttemptsUsed:     0,
			MaxAttempts:      maxSearchCalls,
			Queries:          nil,
			SourcesCount:     0,
			FollowUpRequired: true,
		})}, nil
	}

	allowedDomains := argStringSlice(args, "allowed_domains")
	blockedDomains := argStringSlice(args, "blocked_domains")
	if len(allowedDomains) > 0 && len(blockedDomains) > 0 {
		msg := "Error: cannot specify both allowed_domains and blocked_domains in the same request"
		return ToolOutcome{Content: msg, Success: false, ErrorMessage: msg, MetadataJSON: marshalWebSearchMetadata(webSearchMetadata{
			Kind:             "web_search",
			CompletionReason: "search_error",
			AttemptsUsed:     0,
			MaxAttempts:      maxSearchCalls,
			Queries:          seedQueries,
			SourcesCount:     0,
			FollowUpRequired: true,
		})}, nil
	}

	client := t.resolveInnerClient()
	if client == nil {
		return t.executeLegacyFallback(ctx, seedQueries, allowedDomains, blockedDomains)
	}

	if err := acquireWebSearchInnerLoopSlot(ctx); err != nil {
		return ToolOutcome{
			Content:      "Search cancelled before execution.",
			Success:      false,
			ErrorMessage: err.Error(),
			MetadataJSON: marshalWebSearchMetadata(webSearchMetadata{
				Kind:             "web_search",
				CompletionReason: "search_error",
				AttemptsUsed:     0,
				MaxAttempts:      maxSearchCalls,
				Queries:          seedQueries,
				SourcesCount:     0,
				FollowUpRequired: true,
			}),
		}, nil
	}
	defer releaseWebSearchInnerLoopSlot()

	return t.executeInnerLoop(ctx, client, seedQueries, allowedDomains, blockedDomains)
}

func (t *WebSearchTool) executeInnerLoop(
	ctx context.Context,
	client llm.Client,
	seedQueries []string,
	allowedDomains []string,
	blockedDomains []string,
) (ToolOutcome, error) {
	startedAt := time.Now()
	useAdapters := shouldUseAdapterPath(allowedDomains)

	toolDefs := []llm.ToolDef{innerSearchToolDef()}
	messages := []llm.Message{
		{
			Role: llm.RoleSystem,
			Content: "You are a research sub-agent. Find authoritative sources using only the provided search(query) tool. " +
				"If results are weak, reformulate queries. Stop when you have at least 3 credible sources or no new signal. " +
				"Do not answer the user question; only drive search exploration.",
		},
		{Role: llm.RoleUser, Content: buildInnerSeedMessage(seedQueries)},
	}

	searchFn := t.searchDispatcher(useAdapters)
	attemptSections := make([]string, 0, maxSearchCalls)
	queriesUsed := make([]string, 0, maxSearchCalls)
	querySeen := make(map[string]struct{}, maxSearchCalls)
	urlSeen := make(map[string]web.SearchResult, 16)
	urlOrder := make([]string, 0, 16)
	searchCalls := 0
	completionReason := "assistant_complete"

outer:
	for turn := 0; turn < maxInnerTurns; turn++ {
		if ctx.Err() != nil {
			completionReason = "search_error"
			break
		}

		resp, err := client.Chat(ctx, messages, toolDefs, nil)
		if err != nil {
			completionReason = "search_error"
			break
		}

		if len(resp.ToolCalls) == 0 {
			completionReason = "assistant_complete"
			break
		}

		messages = append(messages, llm.Message{Role: llm.RoleAssistant, Content: resp.Content, ToolCalls: resp.ToolCalls})

		for _, tc := range resp.ToolCalls {
			if tc.Function.Name != "search" {
				continue
			}
			query := strings.TrimSpace(parseInnerSearchQuery(tc.Function.Arguments))
			if query == "" {
				messages = append(messages, llm.Message{Role: llm.RoleTool, ToolCallID: tc.ID, Content: `{"error":"missing query"}`})
				continue
			}

			normQ := strings.ToLower(query)
			if _, dup := querySeen[normQ]; dup {
				messages = append(messages, llm.Message{Role: llm.RoleTool, ToolCallID: tc.ID, Content: `{"deduped":true}`})
				continue
			}
			querySeen[normQ] = struct{}{}
			queriesUsed = append(queriesUsed, query)

			searchCalls++
			EmitOutput(ctx, fmt.Sprintf("attempt=%d/%d query=%q", searchCalls, maxSearchCalls, query))

			searchStarted := time.Now()
			results, err := searchFn(ctx, query, webSearchMaxUses, allowedDomains, blockedDomains)
			elapsed := time.Since(searchStarted)
			if err != nil {
				messages = append(messages, llm.Message{Role: llm.RoleTool, ToolCallID: tc.ID, Content: fmt.Sprintf(`{"error":%q}`, err.Error())})
			} else {
				attemptSections = append(attemptSections, strings.TrimSpace(formatWebSearchResults(query, results, elapsed)))
				messages = append(messages, llm.Message{Role: llm.RoleTool, ToolCallID: tc.ID, Content: formatInnerSearchToolResult(query, results)})
				for _, r := range results {
					url := strings.TrimSpace(r.URL)
					if url == "" {
						continue
					}
					if _, exists := urlSeen[url]; exists {
						continue
					}
					urlSeen[url] = r
					urlOrder = append(urlOrder, url)
				}
			}

			if searchCalls >= maxSearchCalls {
				if len(urlOrder) == 0 {
					completionReason = "budget_exhausted_no_sources"
				} else {
					completionReason = "budget_exhausted_with_sources"
				}
				break outer
			}
		}
	}

	if ctx.Err() != nil {
		completionReason = "search_error"
	}
	if searchCalls >= maxSearchCalls && completionReason == "assistant_complete" {
		if len(urlOrder) == 0 {
			completionReason = "budget_exhausted_no_sources"
		} else {
			completionReason = "budget_exhausted_with_sources"
		}
	}
	if completionReason == "assistant_complete" && searchCalls > 0 && len(urlOrder) == 0 {
		completionReason = "no_results"
	}

	content := composeInnerSearchContent(attemptSections, urlOrder, urlSeen)
	if content == "" {
		content = "No results found for the provided queries."
	}

	meta := webSearchMetadata{
		Kind:             "web_search",
		CompletionReason: completionReason,
		AttemptsUsed:     searchCalls,
		MaxAttempts:      maxSearchCalls,
		Queries:          queriesUsed,
		SourcesCount:     len(urlOrder),
		FollowUpRequired: len(urlOrder) == 0 || completionReason == "no_results" || completionReason == "search_error" || completionReason == "budget_exhausted_no_sources",
	}

	slog.Info("web_search inner done",
		"attempts", searchCalls,
		"reason", completionReason,
		"sources", len(urlOrder),
		"duration", time.Since(startedAt),
	)

	return ToolOutcome{
		Content:      content,
		Success:      true,
		MetadataJSON: marshalWebSearchMetadata(meta),
	}, nil
}

func (t *WebSearchTool) executeLegacyFallback(
	ctx context.Context,
	queries []string,
	allowedDomains []string,
	blockedDomains []string,
) (ToolOutcome, error) {
	searchFn := t.SearchFn
	if searchFn == nil {
		searchFn = web.Search
	}

	var wg sync.WaitGroup
	resultsChan := make(chan queryResult, len(queries))

	for _, q := range queries {
		wg.Add(1)
		go func(query string) {
			defer wg.Done()
			start := time.Now()
			res, err := searchFn(
				ctx,
				query,
				webSearchMaxUses,
				web.WithAllowedDomains(allowedDomains...),
				web.WithBlockedDomains(blockedDomains...),
			)
			resultsChan <- queryResult{query: query, results: res, duration: time.Since(start), err: err}
		}(q)
	}

	wg.Wait()
	close(resultsChan)

	var sb strings.Builder
	var firstErr error
	totalResults := 0
	urlSeen := make(map[string]struct{}, 16)
	queriesUsed := make([]string, 0, len(queries))

	for res := range resultsChan {
		queriesUsed = append(queriesUsed, res.query)
		if res.err != nil {
			if firstErr == nil {
				firstErr = res.err
			}
			continue
		}
		totalResults += len(res.results)
		for _, item := range res.results {
			if u := strings.TrimSpace(item.URL); u != "" {
				urlSeen[u] = struct{}{}
			}
		}
		sb.WriteString(formatWebSearchResults(res.query, res.results, res.duration))
		sb.WriteString("\n\n---\n\n")
	}

	if totalResults == 0 && firstErr != nil {
		msg := fmt.Sprintf("web search failed: %v", firstErr)
		meta := webSearchMetadata{
			Kind:             "web_search",
			CompletionReason: "legacy_fallback",
			AttemptsUsed:     len(queriesUsed),
			MaxAttempts:      maxSearchCalls,
			Queries:          queriesUsed,
			SourcesCount:     0,
			FollowUpRequired: true,
		}
		return ToolOutcome{Content: msg, Success: false, ErrorMessage: msg, MetadataJSON: marshalWebSearchMetadata(meta)}, nil
	}

	content := strings.TrimSpace(sb.String())
	if content == "" {
		content = "No results found for the provided queries."
	}

	meta := webSearchMetadata{
		Kind:             "web_search",
		CompletionReason: "legacy_fallback",
		AttemptsUsed:     len(queriesUsed),
		MaxAttempts:      maxSearchCalls,
		Queries:          queriesUsed,
		SourcesCount:     len(urlSeen),
		FollowUpRequired: len(urlSeen) == 0,
	}

	return ToolOutcome{
		Content:      content,
		Success:      true,
		MetadataJSON: marshalWebSearchMetadata(meta),
	}, nil
}

func (t *WebSearchTool) searchDispatcher(useAdapters bool) func(context.Context, string, int, []string, []string) ([]web.SearchResult, error) {
	return func(ctx context.Context, query string, limit int, allowedDomains, blockedDomains []string) ([]web.SearchResult, error) {
		opts := []web.SearchOption{web.WithAllowedDomains(allowedDomains...), web.WithBlockedDomains(blockedDomains...)}
		if useAdapters {
			searchFn := t.SearchFn
			if searchFn == nil {
				searchFn = web.Search
			}
			return searchFn(ctx, query, limit, opts...)
		}
		rawFn := t.RawSearchFn
		if rawFn == nil {
			rawFn = web.RawSearch
		}
		return rawFn(ctx, query, limit, opts...)
	}
}

func (t *WebSearchTool) resolveInnerClient() llm.Client {
	if t.LLMRegistry != nil {
		if c, _, err := t.LLMRegistry.Resolve(llm.TierFast); err == nil {
			return c
		}
		if c, _, err := t.LLMRegistry.Resolve(llm.TierGeneral); err == nil {
			return c
		}
	}
	return t.LLMClient
}

func collectSeedQueries(args map[string]interface{}) []string {
	queries := make([]string, 0, 4)
	if qList, ok := args["queries"].([]interface{}); ok {
		for _, q := range qList {
			if s, ok := q.(string); ok {
				trimmed := strings.TrimSpace(s)
				if trimmed != "" {
					queries = append(queries, trimmed)
				}
			}
		}
	}
	if q, ok := args["query"].(string); ok {
		trimmed := strings.TrimSpace(q)
		if trimmed != "" {
			queries = append(queries, trimmed)
		}
	}
	return deduplicate(queries)
}

func shouldUseAdapterPath(allowedDomains []string) bool {
	if len(allowedDomains) == 0 {
		return false
	}
	for _, d := range allowedDomains {
		norm := normalizeDomain(d)
		if norm == "" {
			continue
		}
		if _, ok := adapterKnownDomains[norm]; ok {
			return true
		}
	}
	return false
}

func normalizeDomain(raw string) string {
	d := strings.TrimSpace(strings.ToLower(raw))
	d = strings.TrimPrefix(d, "http://")
	d = strings.TrimPrefix(d, "https://")
	d = strings.TrimPrefix(d, "www.")
	d = strings.TrimPrefix(d, "site:")
	d = strings.TrimPrefix(d, "-site:")
	d = strings.Trim(d, "/")
	if i := strings.Index(d, "/"); i >= 0 {
		d = d[:i]
	}
	return d
}

var adapterKnownDomains = map[string]struct{}{
	"github.com":         {},
	"huggingface.co":     {},
	"hub.docker.com":     {},
	"pypi.org":           {},
	"npmjs.com":          {},
	"registry.npmjs.org": {},
	"osv.dev":            {},
	"api.osv.dev":        {},
}

func innerSearchToolDef() llm.ToolDef {
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "search",
			Description: "Search the web for one query and inspect the results.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Single search query",
					},
				},
				"required": []string{"query"},
			},
		},
	}
}

func buildInnerSeedMessage(seeds []string) string {
	if len(seeds) == 0 {
		return "Research this topic. Reformulate queries until you gather authoritative sources."
	}
	var sb strings.Builder
	sb.WriteString("Seed queries (use if useful, ignore if weak):\n")
	for _, q := range seeds {
		sb.WriteString("- ")
		sb.WriteString(q)
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}

func parseInnerSearchQuery(arguments string) string {
	var payload struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(arguments), &payload); err == nil {
		return payload.Query
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(arguments), &raw); err != nil {
		return ""
	}
	if q, ok := raw["query"].(string); ok {
		return q
	}
	return ""
}

func formatInnerSearchToolResult(query string, results []web.SearchResult) string {
	if len(results) == 0 {
		return fmt.Sprintf(`{"query":%q,"count":0}`, query)
	}
	type row struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Snippet string `json:"snippet,omitempty"`
	}
	rows := make([]row, 0, len(results))
	for _, r := range results {
		rows = append(rows, row{Title: strings.TrimSpace(nonEmpty(r.Title, r.URL)), URL: strings.TrimSpace(r.URL), Snippet: strings.TrimSpace(r.Snippet)})
	}
	payload := map[string]any{"query": query, "count": len(rows), "results": rows}
	b, _ := json.Marshal(payload)
	return string(b)
}

func composeInnerSearchContent(attemptSections []string, urlOrder []string, byURL map[string]web.SearchResult) string {
	parts := make([]string, 0, 2)
	if len(attemptSections) > 0 {
		parts = append(parts, strings.Join(attemptSections, "\n\n---\n\n"))
	}
	if len(urlOrder) > 0 {
		var sb strings.Builder
		sb.WriteString("Sources:\n")
		for _, u := range urlOrder {
			r := byURL[u]
			title := markdownSafeText(nonEmpty(r.Title, r.URL))
			snippet := strings.TrimSpace(r.Snippet)
			if snippet != "" {
				fmt.Fprintf(&sb, "- [%s](%s): %s\n", title, r.URL, snippet)
			} else {
				fmt.Fprintf(&sb, "- [%s](%s)\n", title, r.URL)
			}
		}
		parts = append(parts, strings.TrimSpace(sb.String()))
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n---\n\n"))
}

func marshalWebSearchMetadata(meta webSearchMetadata) string {
	if meta.Queries == nil {
		meta.Queries = []string{}
	}
	b, _ := json.Marshal(meta)
	return string(b)
}

func acquireWebSearchInnerLoopSlot(ctx context.Context) error {
	select {
	case webSearchInnerLoopSem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func releaseWebSearchInnerLoopSlot() {
	select {
	case <-webSearchInnerLoopSem:
	default:
	}
}

func deduplicate(in []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, s := range in {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

func formatWebSearchResults(query string, results []web.SearchResult, duration time.Duration) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Web search results for query: %q\n", query))
	sb.WriteString(fmt.Sprintf("Found %d results in %s\n\n", len(results), formatSearchDuration(duration)))

	if len(results) > 0 {
		sb.WriteString("Sources:\n")
		for _, r := range results {
			title := markdownSafeText(nonEmpty(r.Title, r.URL))
			if r.Snippet != "" {
				fmt.Fprintf(&sb, "- [%s](%s): %s\n", title, r.URL, r.Snippet)
			} else {
				fmt.Fprintf(&sb, "- [%s](%s)\n", title, r.URL)
			}
		}
	}

	return sb.String()
}

func formatSearchDuration(d time.Duration) string {
	if d >= time.Second {
		return fmt.Sprintf("%ds", int(d.Round(time.Second)/time.Second))
	}
	return fmt.Sprintf("%dms", int(d.Round(time.Millisecond)/time.Millisecond))
}

func markdownSafeText(in string) string {
	s := strings.ReplaceAll(in, "[", "(")
	s = strings.ReplaceAll(s, "]", ")")
	return strings.TrimSpace(s)
}

func nonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
