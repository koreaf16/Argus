package llm

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestFormatRequestLogEntryRendersPromptAsReadableText(t *testing.T) {
	req := Request{
		Model: "test-model",
		System: []SystemBlock{
			{Type: "text", Text: "system rules"},
		},
		Messages: []Message{
			TextMessage(RoleUser, "check postgres"),
			{
				Role: RoleAssistant,
				Content: []ContentBlock{
					{Type: ContentToolUse, ID: "tool-1", Name: "bash", Input: json.RawMessage(`{"command":"ps -ef | grep postgres","server":"sandbox-server"}`)},
				},
			},
			{
				Role: RoleUser,
				Content: []ContentBlock{
					{Type: ContentToolResult, ToolUseID: "tool-1", Name: "bash", Text: "postgres is running", IsError: false},
				},
			},
		},
		Tools: []ToolSpec{
			{
				Name:        "bash",
				Description: "Execute bash commands",
				InputSchema: map[string]any{
					"type":     "object",
					"required": []string{"command"},
					"properties": map[string]any{
						"command": map[string]any{"type": "string"},
					},
				},
			},
		},
		MaxTokens: 256,
	}

	got := formatRequestLogEntry("provider", "test-model", req, time.Date(2026, 5, 2, 1, 2, 3, 0, time.UTC))

	for _, want := range []string{
		requestLogSeparator,
		"LLM REQUEST",
		"Provider: provider",
		"Model: test-model",
		"SYSTEM PROMPT",
		"system rules",
		"CONVERSATION MESSAGES",
		"Tool call:",
		"name: bash",
		"command: ps -ef | grep postgres",
		"server: sandbox-server",
		"Tool result:",
		"postgres is running",
		"AVAILABLE TOOLS",
		"input schema:",
		"required:",
		"- command",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted log missing %q:\n%s", want, got)
		}
	}

	if strings.Contains(got, `{"command"`) || strings.Contains(got, `"server"`) {
		t.Fatalf("formatted log should not include raw JSON object syntax:\n%s", got)
	}
}

func TestFormatRequestLogEntryPreservesUTF8Text(t *testing.T) {
	req := Request{
		System: []SystemBlock{
			{Type: "text", Text: "CONNECTION INTENT: 접속해, X로 바꿔, X 워크스페이스로"},
		},
		Messages: []Message{
			TextMessage(RoleUser, "sandbox-server 접속해"),
			{
				Role: RoleUser,
				Content: []ContentBlock{
					{
						Type:      ContentToolResult,
						ToolUseID: "tool-1",
						Name:      "server_connect",
						Text:      "나의 sandbox-server에 접속되었습니다. 무엇을 도와드릴까요?",
					},
				},
			},
		},
		Tools: []ToolSpec{
			{Name: "server_connect", Description: "Connect to a registered workspace"},
		},
	}

	got := formatRequestLogEntry("provider", "model", req, time.Date(2026, 5, 2, 1, 2, 3, 0, time.UTC))

	for _, want := range []string{
		"접속해",
		"X로 바꿔",
		"X 워크스페이스로",
		"sandbox-server 접속해",
		"나의 sandbox-server에 접속되었습니다",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted log lost UTF-8 text %q:\n%s", want, got)
		}
	}
}
