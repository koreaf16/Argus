package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yourorg/infractl/internal/web"
)

type fetchMeta struct {
	Kind             string `json:"kind"`
	FinalURL         string `json:"final_url"`
	RedirectBlocked  bool   `json:"redirect_blocked"`
	ContentBytes     int    `json:"content_bytes"`
	FollowUpRequired bool   `json:"follow_up_required"`
	FollowUpURL      string `json:"follow_up_url"`
}

func decodeFetchMeta(t *testing.T, raw string) fetchMeta {
	t.Helper()
	var m fetchMeta
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	return m
}

func TestWebFetch_RedirectBlocked_EmitsFollowUp(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>target</body></html>`))
	}))
	defer target.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/doc", http.StatusFound)
	}))
	defer source.Close()

	tool := &WebFetchTool{Fetcher: web.NewFetcher(16, 0), LLMClient: stubChatClient{response: "processed output"}}
	out, err := tool.Execute(context.Background(), map[string]interface{}{"url": source.URL + "/start", "prompt": "extract"}, webFetchNoopExecutor{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	meta := decodeFetchMeta(t, out.MetadataJSON)
	if !meta.RedirectBlocked || !meta.FollowUpRequired {
		t.Fatalf("expected redirect blocked + follow-up, got %+v", meta)
	}
	if meta.FollowUpURL == "" {
		t.Fatal("expected follow_up_url for redirect")
	}
}

func TestWebFetch_ShortContent_FollowUpRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><p>short</p></body></html>`))
	}))
	defer srv.Close()

	tool := &WebFetchTool{Fetcher: web.NewFetcher(16, 0), LLMClient: stubChatClient{response: "processed"}}
	out, err := tool.Execute(context.Background(), map[string]interface{}{"url": srv.URL, "prompt": "extract"}, webFetchNoopExecutor{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	meta := decodeFetchMeta(t, out.MetadataJSON)
	if !meta.FollowUpRequired {
		t.Fatalf("expected follow_up_required=true for short content, got %+v", meta)
	}
}

func TestWebFetch_FetchError_FollowUpRequired(t *testing.T) {
	tool := &WebFetchTool{}
	out, err := tool.Execute(context.Background(), map[string]interface{}{"url": "http://127.0.0.1:1", "prompt": "extract"}, webFetchNoopExecutor{})
	if err != nil {
		t.Fatalf("Execute should not return error in fetch-failure path: %v", err)
	}
	if out.Success {
		t.Fatalf("expected failure outcome, got success")
	}
	meta := decodeFetchMeta(t, out.MetadataJSON)
	if !meta.FollowUpRequired {
		t.Fatalf("expected follow_up_required=true on fetch failure, got %+v", meta)
	}
}

func TestWebFetch_HappyPath_NoFollowUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><h1>Long</h1><p>abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz</p></body></html>`))
	}))
	defer srv.Close()

	tool := &WebFetchTool{Fetcher: web.NewFetcher(16, 0), LLMClient: stubChatClient{response: "processed output with enough length to avoid follow-up"}}
	out, err := tool.Execute(context.Background(), map[string]interface{}{"url": srv.URL, "prompt": "extract"}, webFetchNoopExecutor{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	meta := decodeFetchMeta(t, out.MetadataJSON)
	if meta.FollowUpRequired {
		t.Fatalf("expected follow_up_required=false, got %+v", meta)
	}
}
