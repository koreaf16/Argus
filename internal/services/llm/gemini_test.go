package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeminiStream_MultipleFunctionCallsInSingleChunk(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, ":streamGenerateContent") {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"functionCall\":{\"name\":\"web_search\",\"args\":{\"query\":\"gemini\"}}},{\"functionCall\":{\"name\":\"webfetch\",\"args\":{\"url\":\"https://example.com\"}}}]}}]}\n\n")
	}))
	defer srv.Close()

	llmClient := NewGemini(ModelEntry{
		ModelID: "gemini-test",
		BaseURL: srv.URL,
	}, "dummy-key", srv.Client())

	stream, err := llmClient.Stream(context.Background(), Request{
		Messages: []Message{TextMessage(RoleUser, "check latest")},
		Tools: []ToolSpec{
			{Name: "web_search"},
			{Name: "webfetch"},
		},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	var toolNames []string
	stopCount := 0
	for evt := range stream {
		switch evt.Kind {
		case EventToolUseStart:
			if evt.ToolUse == nil {
				t.Fatalf("tool use event missing payload")
			}
			toolNames = append(toolNames, evt.ToolUse.Name)
		case EventStop:
			if evt.Stop != nil && *evt.Stop == StopReasonToolUse {
				stopCount++
			}
		case EventError:
			t.Fatalf("unexpected stream error: %v", evt.Err)
		}
	}

	if len(toolNames) != 2 {
		t.Fatalf("expected 2 tool uses, got %d (%v)", len(toolNames), toolNames)
	}
	if toolNames[0] != "web_search" || toolNames[1] != "webfetch" {
		t.Fatalf("unexpected tool use order: %v", toolNames)
	}
	if stopCount != 1 {
		t.Fatalf("expected one tool_use stop event, got %d", stopCount)
	}
}
