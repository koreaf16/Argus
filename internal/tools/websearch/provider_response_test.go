package websearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHuggingFaceProvider_Search_HandlesModelIDAlias(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(httpHandlerJSON(`[
		{"modelId":"Qwen/Qwen2.5-7B-Instruct","likes":12,"downloads":1000,"pipeline_tag":"text-generation"}
	]`)))
	defer srv.Close()

	provider := NewHuggingFaceProvider(srv.Client(), srv.URL, nil)
	results, err := provider.Search(context.Background(), "qwen", 5)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Title != "Qwen/Qwen2.5-7B-Instruct" {
		t.Fatalf("unexpected title: %s", results[0].Title)
	}
	if !strings.Contains(results[0].Snippet, "likes=12") || !strings.Contains(results[0].Snippet, "downloads=1000") {
		t.Fatalf("unexpected snippet: %s", results[0].Snippet)
	}
}

func TestDockerHubProvider_Search_HandlesSummariesFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(httpHandlerJSON(`{
		"summaries":[
			{"name":"nginx","short_description":"web server","star_count":1,"pull_count":2}
		]
	}`)))
	defer srv.Close()

	provider := NewDockerHubProvider(srv.Client(), srv.URL, nil)
	results, err := provider.Search(context.Background(), "nginx", 5)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Title != "library/nginx" {
		t.Fatalf("unexpected title: %s", results[0].Title)
	}
	if !strings.Contains(results[0].Snippet, "stars=1") || !strings.Contains(results[0].Snippet, "pulls=2") {
		t.Fatalf("unexpected snippet: %s", results[0].Snippet)
	}
}

func TestArtifactHubProvider_Search_HandlesDataShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(httpHandlerJSON(`{
		"data":[
			{
				"name":"nginx",
				"description":"nginx chart",
				"kind":"helm",
				"version":"1.2.3",
				"repository":{"name":"bitnami"}
			}
		]
	}`)))
	defer srv.Close()

	provider := NewArtifactHubProvider(srv.Client(), srv.URL, nil)
	results, err := provider.Search(context.Background(), "nginx", 5)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Title != "bitnami/nginx" {
		t.Fatalf("unexpected title: %s", results[0].Title)
	}
	if !strings.Contains(results[0].Snippet, "kind=helm") || !strings.Contains(results[0].Snippet, "version=1.2.3") {
		t.Fatalf("unexpected snippet: %s", results[0].Snippet)
	}
}

func TestNuGetProvider_Search_FormatsSnippet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(httpHandlerJSON(`{
		"data":[
			{
				"id":"Serilog",
				"version":"3.1.0",
				"description":"Simple logging",
				"totalDownloads":12345,
				"verified":true
			}
		]
	}`)))
	defer srv.Close()

	provider := NewNuGetProvider(srv.Client(), srv.URL, nil)
	results, err := provider.Search(context.Background(), "serilog", 5)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !strings.Contains(results[0].Snippet, "version=3.1.0") ||
		!strings.Contains(results[0].Snippet, "downloads=12345") ||
		!strings.Contains(results[0].Snippet, "verified") {
		t.Fatalf("unexpected snippet: %s", results[0].Snippet)
	}
}

func TestExtractPyPICandidates_QuotedAndInstallPatterns(t *testing.T) {
	got := extractPyPICandidates(`pip install "fastapi" and check package 'uvicorn'`, 10)
	joined := strings.Join(got, ",")
	if !strings.Contains(joined, "fastapi") {
		t.Fatalf("expected fastapi in candidates: %#v", got)
	}
	if !strings.Contains(joined, "uvicorn") {
		t.Fatalf("expected uvicorn in candidates: %#v", got)
	}
}

func httpHandlerJSON(body string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}
