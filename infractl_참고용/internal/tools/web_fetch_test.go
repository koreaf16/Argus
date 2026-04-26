package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/web"
)

type webFetchNoopExecutor struct{}

func (webFetchNoopExecutor) Execute(context.Context, string) (executor.ExecResult, error) {
	return executor.ExecResult{}, nil
}
func (webFetchNoopExecutor) Target() string { return "localhost" }
func (webFetchNoopExecutor) Host() string   { return "localhost" }

type stubChatClient struct {
	response string
}

func (s stubChatClient) Chat(context.Context, []llm.Message, []llm.ToolDef, interface{}, ...llm.CallOption) (llm.Response, error) {
	return llm.Response{Content: s.response}, nil
}
func (s stubChatClient) ChatStream(context.Context, []llm.Message, []llm.ToolDef, interface{}, func(string), func(string), ...llm.CallOption) (llm.Response, error) {
	return llm.Response{Content: s.response}, nil
}

func TestWebFetchToolRejectsLegacyUrlsField(t *testing.T) {
	tool := &WebFetchTool{}
	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"urls":   []interface{}{"https://example.com/a", "https://example.com/b"},
		"prompt": "summarize",
	}, webFetchNoopExecutor{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if out.Success {
		t.Fatalf("expected failure for legacy urls field")
	}
	if !strings.Contains(out.Content, "single url only") {
		t.Fatalf("unexpected error: %s", out.Content)
	}
}

func TestWebFetchToolAppliesPromptToSingleURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><h1>Title</h1><p>body</p></body></html>`))
	}))
	defer srv.Close()

	tool := &WebFetchTool{
		Fetcher:   web.NewFetcher(16, 0),
		LLMClient: stubChatClient{response: "processed output"},
	}

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"url":    srv.URL,
		"prompt": "extract title",
	}, webFetchNoopExecutor{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !out.Success {
		t.Fatalf("expected success, got failure: %s", out.Content)
	}
	if !strings.Contains(out.Content, "processed output") {
		t.Fatalf("expected processed output, got:\n%s", out.Content)
	}
}

func TestWebFetchToolCrossHostRedirectReturnsInstruction(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>target</body></html>`))
	}))
	defer target.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/doc", http.StatusFound)
	}))
	defer source.Close()

	tool := &WebFetchTool{
		Fetcher:   web.NewFetcher(16, 0),
		LLMClient: stubChatClient{response: "processed output"},
	}
	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"url":    source.URL + "/start",
		"prompt": "extract",
	}, webFetchNoopExecutor{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !out.Success {
		t.Fatalf("expected success redirect instruction, got failure: %s", out.Content)
	}
	if !strings.Contains(out.Content, "REDIRECT DETECTED") {
		t.Fatalf("expected redirect message, got:\n%s", out.Content)
	}
	if !strings.Contains(out.Content, target.URL+"/doc") {
		t.Fatalf("expected redirect URL in output, got:\n%s", out.Content)
	}
}

func TestWebFetchToolPermitsWwwRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/final", http.StatusMovedPermanently)
		default:
			_, _ = w.Write([]byte(`<html><body><h1>Final</h1></body></html>`))
		}
	}))
	defer srv.Close()

	tool := &WebFetchTool{
		Fetcher:   web.NewFetcher(16, 0),
		LLMClient: stubChatClient{response: "final output"},
	}
	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"url":    srv.URL + "/start",
		"prompt": "extract",
	}, webFetchNoopExecutor{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !out.Success {
		t.Fatalf("expected success, got failure: %s", out.Content)
	}
	if strings.Contains(out.Content, "REDIRECT DETECTED") {
		t.Fatalf("expected redirect to be followed, got:\n%s", out.Content)
	}
	if !strings.Contains(out.Content, "final output") {
		t.Fatalf("expected processed output, got:\n%s", out.Content)
	}
}

func TestWebFetchToolParametersSingleURLOnly(t *testing.T) {
	props, _ := (&WebFetchTool{}).Parameters()["properties"].(map[string]interface{})
	if _, ok := props["urls"]; ok {
		t.Fatalf("legacy urls field should not be exposed")
	}
	if _, ok := props["max_length"]; ok {
		t.Fatalf("legacy max_length field should not be exposed")
	}
}
