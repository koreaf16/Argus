package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/web"
)

func TestWebSearchToolExecuteReturnsSourcesAndSnippets(t *testing.T) {
	tool := &WebSearchTool{
		SearchFn: func(context.Context, string, int, ...web.SearchOption) ([]web.SearchResult, error) {
			return []web.SearchResult{
				{Title: "Doc A", URL: "https://example.com/a", Snippet: "Snippet A"},
				{Title: "Doc B", URL: "https://example.com/b", Snippet: "Snippet B"},
			}, nil
		},
	}

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"query": "kubernetes upgrade docs",
	}, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !out.Success {
		t.Fatalf("expected success, got failure: %s", out.Content)
	}
	for _, want := range []string{
		`Web search results for query: "kubernetes upgrade docs"`,
		"Sources:",
		"[Doc A](https://example.com/a): Snippet A",
		"[Doc B](https://example.com/b): Snippet B",
	} {
		if !strings.Contains(out.Content, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, out.Content)
		}
	}
}

func TestWebSearchToolExecuteRejectsAllowedAndBlockedTogether(t *testing.T) {
	tool := &WebSearchTool{
		SearchFn: func(context.Context, string, int, ...web.SearchOption) ([]web.SearchResult, error) {
			t.Fatal("SearchFn should not be called when filters are invalid")
			return nil, nil
		},
	}

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"query":           "test",
		"allowed_domains": []interface{}{"example.com"},
		"blocked_domains": []interface{}{"bad.com"},
	}, webFetchNoopExecutor{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if out.Success {
		t.Fatalf("expected failure when both filters provided")
	}
	if !strings.Contains(out.Content, "cannot specify both allowed_domains and blocked_domains") {
		t.Fatalf("unexpected error: %s", out.Content)
	}
}

func TestWebSearchToolParametersDoNotExposeLegacyFields(t *testing.T) {
	props, _ := (&WebSearchTool{}).Parameters()["properties"].(map[string]interface{})
	for _, legacy := range []string{"max_results", "auto_fetch", "fetch_top_n"} {
		if _, ok := props[legacy]; ok {
			t.Fatalf("legacy field %q should not be exposed", legacy)
		}
	}
}
