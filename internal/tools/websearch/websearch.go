package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/webconfig"
	"github.com/koreaf16/argus/internal/types"
)

const (
	ToolName            = "web_search"
	defaultTimeout      = 15 * time.Second
	defaultBufferLength = 2
)

type SearchInput struct {
	Query          string   `json:"query"`
	AllowedDomains []string `json:"allowed_domains,omitempty"`
	BlockedDomains []string `json:"blocked_domains,omitempty"`
}

type SearchOutput struct {
	Query           string  `json:"query"`
	Results         []any   `json:"results"`
	DurationSeconds float64 `json:"durationSeconds"`
}

type WebSearchTool struct {
	httpClient *http.Client
	client     *SearXNGClient
	router     *SearchRouter // non-nil only when set directly (tests)
	maxResult  int
}

func NewWebSearchTool() *WebSearchTool {
	return NewWebSearchToolWithClient(&http.Client{Timeout: defaultTimeout})
}

func NewWebSearchToolWithClient(httpClient *http.Client) *WebSearchTool {
	cfg := webconfig.Load()
	client := NewSearXNGClient(httpClient)
	client.SetEndpoint(cfg.SearXNGBaseURL)
	return &WebSearchTool{
		httpClient: httpClient,
		client:     client,
		maxResult:  cfg.SearXNGMax,
	}
}

func (t *WebSearchTool) Name() string {
	return ToolName
}

func (t *WebSearchTool) Description(ctx tool.Context) string {
	_ = ctx
	return GetWebSearchDescription()
}

func (t *WebSearchTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"minLength":   2,
				"description": "The search query to use. Start with broad terms and avoid overly specific parameters unless necessary.",
			},
			"allowed_domains": map[string]any{
				"type":        "array",
				"description": "Only include search results from these domains",
				"items": map[string]any{
					"type": "string",
				},
			},
			"blocked_domains": map[string]any{
				"type":        "array",
				"description": "Never include search results from these domains",
				"items": map[string]any{
					"type": "string",
				},
			},
		},
		"required":             []string{"query"},
		"additionalProperties": false,
	}
}

func (t *WebSearchTool) IsReadOnly() bool {
	return true
}

func (t *WebSearchTool) MaxResultSizeChars() int {
	return 30000
}

func (t *WebSearchTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	_ = ctx
	_ = input
	return types.PermissionResult{
		Behavior: types.BehaviorAllow,
		Message:  "web_search is read-only",
	}, nil
}

func (t *WebSearchTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	parsed, err := parseAndValidateInput(input)
	if err != nil {
		return nil, err
	}

	out := make(chan tool.ToolEvent, defaultBufferLength)
	go func() {
		defer close(out)

		callCtx := ctx.Context
		if callCtx == nil {
			callCtx = context.Background()
		}

		router := t.router
		if router == nil {
			router = NewDefaultSearchRouter(t.httpClient, ctx.ExecuteSubQuery)
		}

		started := time.Now()
		rawResults := []SearchResult(nil)
		var searchErr error
		providerNoResults := false // true when provider succeeded but returned 0 results

		if router != nil {
			if provider, score := router.RouteWithScore(parsed); provider != nil {
				rawResults, searchErr = provider.Search(callCtx, parsed.Query, t.maxResult)
				if searchErr == nil && len(rawResults) == 0 {
					// Provider call succeeded but returned nothing.
					// For a strong brand/domain match, report empty results rather than
					// surfacing unrelated general-web content via SearXNG.
					providerNoResults = score >= brandMatchScore
					if !providerNoResults {
						searchErr = fmt.Errorf("no provider results")
					}
				}
			}
		}

		if providerNoResults {
			payload := SearchOutput{
				Query:           parsed.Query,
				DurationSeconds: time.Since(started).Seconds(),
			}
			payload.Results = append(payload.Results, "지정된 사이트에서 결과를 찾을 수 없습니다. 더 넓은 검색어나 다른 쿼리를 시도해 보세요.")
			encoded, _ := json.Marshal(payload)
			out <- tool.NewOutputEvent(string(encoded))
			out <- tool.NewDoneEvent()
			return
		}

		// Fall back to SearXNG on provider error or weak/no match.
		if searchErr != nil || len(rawResults) == 0 {
			rawResults, searchErr = t.client.Search(callCtx, parsed.Query, t.maxResult)
			if searchErr != nil {
				out <- tool.NewErrorEvent(searchErr)
				out <- tool.NewDoneEvent()
				return
			}
		}

		filtered := ApplyDomainFilters(rawResults, parsed.AllowedDomains, parsed.BlockedDomains)
		payload := SearchOutput{
			Query:           parsed.Query,
			DurationSeconds: time.Since(started).Seconds(),
		}

		if len(filtered) == 0 {
			payload.Results = append(payload.Results, "결과를 찾을 수 없습니다. 더 넓은 용어나 다른 키워드로 다시 시도하십시오.")
		} else {
			payload.Results = append(payload.Results, fmt.Sprintf(
				"%d개의 결과를 찾았습니다. 선택한 URL과 추출 프롬프트와 함께 webfetch 도구를 사용하여 페이지 콘텐츠를 읽으십시오.",
				len(filtered),
			))
			payload.Results = append(payload.Results, map[string]any{
				"tool_use_id": "websearch-" + time.Now().Format("150405"),
				"content":     filtered,
			})
		}

		encoded, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			out <- tool.NewErrorEvent(marshalErr)
			out <- tool.NewDoneEvent()
			return
		}

		out <- tool.NewOutputEvent(string(encoded))
		out <- tool.NewDoneEvent()
	}()

	return out, nil
}

func parseAndValidateInput(input json.RawMessage) (SearchInput, error) {
	var parsed SearchInput
	if err := json.Unmarshal(input, &parsed); err != nil {
		return SearchInput{}, fmt.Errorf("invalid web_search input: %w", err)
	}

	parsed.Query = strings.TrimSpace(parsed.Query)
	if len(parsed.Query) < 2 {
		return SearchInput{}, fmt.Errorf("query must be at least 2 characters")
	}
	if len(parsed.AllowedDomains) > 0 && len(parsed.BlockedDomains) > 0 {
		return SearchInput{}, fmt.Errorf("cannot use allowed_domains and blocked_domains together")
	}
	return parsed, nil
}
