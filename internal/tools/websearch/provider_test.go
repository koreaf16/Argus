package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tool "github.com/koreaf16/argus/internal/tools"
)

func TestSearchRouter_RouteSelection(t *testing.T) {
	router := NewDefaultSearchRouter(&http.Client{}, nil)

	cases := []struct {
		name  string
		input SearchInput
		want  string
	}{
		{
			name:  "allowed domain prefers huggingface",
			input: SearchInput{Query: "qwen 27b", AllowedDomains: []string{"huggingface.co"}},
			want:  "huggingface",
		},
		{
			name:  "allowed domain prefers docker hub",
			input: SearchInput{Query: "nginx image", AllowedDomains: []string{"hub.docker.com"}},
			want:  "dockerhub",
		},
		{
			name:  "allowed domain prefers pypi",
			input: SearchInput{Query: "requests package", AllowedDomains: []string{"pypi.org"}},
			want:  "pypi",
		},
		{
			name:  "allowed domain prefers npm",
			input: SearchInput{Query: "axios package", AllowedDomains: []string{"npmjs.com"}},
			want:  "npm",
		},
		{
			name:  "allowed domain prefers maven",
			input: SearchInput{Query: "spring dependency", AllowedDomains: []string{"search.maven.org"}},
			want:  "mavencentral",
		},
		{
			name:  "allowed domain prefers nuget",
			input: SearchInput{Query: "serilog package", AllowedDomains: []string{"nuget.org"}},
			want:  "nuget",
		},
		{
			name:  "allowed domain prefers artifacthub",
			input: SearchInput{Query: "nginx chart", AllowedDomains: []string{"artifacthub.io"}},
			want:  "artifacthub",
		},
		{
			name:  "ambiguous brand mention falls back",
			input: SearchInput{Query: "find this on hugging face and github"},
			want:  "",
		},
		{
			name:  "blocked domain skips provider",
			input: SearchInput{Query: "find qwen on huggingface", BlockedDomains: []string{"huggingface.co"}},
			want:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := router.Route(tc.input)
			if tc.want == "" {
				if got != nil {
					t.Fatalf("expected nil provider, got %s", got.Name())
				}
				return
			}
			if got == nil {
				t.Fatalf("expected provider %q, got nil", tc.want)
			}
			if got.Name() != tc.want {
				t.Fatalf("expected provider %q, got %q", tc.want, got.Name())
			}
		})
	}
}

func TestWebSearchTool_FallbackWhenProviderFails(t *testing.T) {
	searxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query": "x",
			"results": []map[string]any{
				{"title": "fallback-result", "url": "https://example.com/fallback", "content": "snippet"},
			},
		})
	}))
	defer searxServer.Close()

	client := NewSearXNGClient(searxServer.Client())
	client.SetEndpoint(searxServer.URL)

	ws := &WebSearchTool{
		client:    client,
		router:    NewSearchRouter(&stubProvider{name: "failing", domain: "failing.test", score: 100, err: errors.New("provider failed")}),
		maxResult: 5,
	}

	stream, err := ws.Call(tool.NewContext(context.Background()), mustInput(t, SearchInput{Query: "anything"}))
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}

	output := collectOutput(t, stream)
	if !strings.Contains(output, "fallback-result") {
		t.Fatalf("expected fallback search result in output, got: %s", output)
	}
}

func TestWebSearchTool_UsesProviderResults(t *testing.T) {
	ws := &WebSearchTool{
		client:    NewSearXNGClient(&http.Client{}), // unused in this test
		router:    NewSearchRouter(&stubProvider{name: "ok", domain: "ok.test", score: 100, results: []SearchResult{{Title: "provider-result", URL: "https://ok.test/r", Snippet: "s"}}}),
		maxResult: 5,
	}

	stream, err := ws.Call(tool.NewContext(context.Background()), mustInput(t, SearchInput{Query: "anything"}))
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}

	output := collectOutput(t, stream)
	if !strings.Contains(output, "provider-result") {
		t.Fatalf("expected provider result in output, got: %s", output)
	}
}

func TestWebSearchTool_BlockedDomainSkipsProvider(t *testing.T) {
	searxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query": "x",
			"results": []map[string]any{
				{"title": "fallback-only", "url": "https://example.com/fallback", "content": "snippet"},
			},
		})
	}))
	defer searxServer.Close()

	client := NewSearXNGClient(searxServer.Client())
	client.SetEndpoint(searxServer.URL)

	ws := &WebSearchTool{
		client: client,
		router: NewSearchRouter(&stubProvider{
			name:    "blocked-provider",
			domain:  "blocked.test",
			score:   100,
			results: []SearchResult{{Title: "should-not-appear", URL: "https://blocked.test/x", Snippet: "x"}},
		}),
		maxResult: 5,
	}

	stream, err := ws.Call(tool.NewContext(context.Background()), mustInput(t, SearchInput{
		Query:          "anything",
		BlockedDomains: []string{"blocked.test"},
	}))
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}

	output := collectOutput(t, stream)
	if strings.Contains(output, "should-not-appear") {
		t.Fatalf("blocked provider result should not appear, got: %s", output)
	}
	if !strings.Contains(output, "fallback-only") {
		t.Fatalf("expected fallback result, got: %s", output)
	}
}

type stubProvider struct {
	name    string
	domain  string
	score   int
	results []SearchResult
	err     error
}

func (s *stubProvider) Name() string {
	return s.name
}

func (s *stubProvider) Domain() string {
	return s.domain
}

func (s *stubProvider) Match(input SearchInput) ProviderMatch {
	_ = input
	return ProviderMatch{Score: s.score}
}

func (s *stubProvider) Search(_ context.Context, _ string, _ int) ([]SearchResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.results, nil
}

func mustInput(t *testing.T, in SearchInput) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	return raw
}

func collectOutput(t *testing.T, stream <-chan tool.ToolEvent) string {
	t.Helper()
	var output strings.Builder
	for evt := range stream {
		switch evt.Kind {
		case tool.ToolEventOutput:
			output.WriteString(evt.Output)
		case tool.ToolEventError:
			if evt.Err != nil {
				t.Fatalf("tool stream error: %v", evt.Err)
			}
		}
	}
	return output.String()
}
