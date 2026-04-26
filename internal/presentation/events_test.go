package presentation

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/koreaf16/argus/internal/query"
	"github.com/koreaf16/argus/internal/services/llm"
	tool "github.com/koreaf16/argus/internal/tools"
)

func TestReplayEvents_BuildsTranscriptRows(t *testing.T) {
	msgs := []llm.Message{
		llm.TextMessage(llm.RoleUser, "hello"),
		{
			Role: llm.RoleAssistant,
			Content: []llm.ContentBlock{
				{Type: llm.ContentText, Text: "hi"},
				{Type: llm.ContentToolUse, Name: "bash", Input: json.RawMessage(`{"command":"pwd"}`)},
			},
		},
		{
			Role: llm.RoleUser,
			Content: []llm.ContentBlock{
				{Type: llm.ContentToolResult, Name: "bash", Text: "ok"},
			},
		},
	}

	got := ReplayEvents(msgs)
	if len(got) != 4 {
		t.Fatalf("expected 4 replay events, got %d", len(got))
	}
	if got[0].Kind != EventUser || got[0].Text != "hello" {
		t.Fatalf("unexpected first event: %#v", got[0])
	}
	if got[1].Kind != EventAssistantText || got[1].Text != "hi" {
		t.Fatalf("unexpected second event: %#v", got[1])
	}
	if got[2].Kind != EventToolUse || got[2].ToolName != "bash" {
		t.Fatalf("unexpected third event: %#v", got[2])
	}
	if got[3].Kind != EventToolResult || got[3].Text != "ok" {
		t.Fatalf("unexpected fourth event: %#v", got[3])
	}
}

func TestFromUIEvent_PasswordPrompt(t *testing.T) {
	evt, ok := FromUIEvent(query.UIEvent{
		Kind:     query.UIEventPasswordPrompt,
		ToolName: "bash",
		Prompt:   "sudo password:",
	})
	if !ok {
		t.Fatalf("expected mapping to succeed")
	}
	if evt.Kind != EventPasswordRequest {
		t.Fatalf("unexpected kind: %s", evt.Kind)
	}
	if evt.ToolName != "bash" || evt.Prompt != "sudo password:" {
		t.Fatalf("unexpected event payload: %#v", evt)
	}
}

func TestFromUIEvent_ThinkingDelta(t *testing.T) {
	evt, ok := FromUIEvent(query.UIEvent{
		Kind:  query.UIEventThinkingDelta,
		Delta: "chain-of-thought",
	})
	if !ok {
		t.Fatalf("expected mapping to succeed")
	}
	if evt.Kind != EventThinkingDelta {
		t.Fatalf("unexpected kind: %s", evt.Kind)
	}
	if evt.Text != "chain-of-thought" {
		t.Fatalf("unexpected text: %q", evt.Text)
	}
}

func TestFromUIEvent_AskUserPrompt(t *testing.T) {
	evt, ok := FromUIEvent(query.UIEvent{
		Kind:     query.UIEventAskUserPrompt,
		ToolName: "ask_user",
		Question: &tool.AskUserQuestion{
			Question: "Pick one",
			Options: []tool.AskUserOption{
				{Label: "A"},
				{Label: "B"},
			},
		},
	})
	if !ok {
		t.Fatalf("expected mapping to succeed")
	}
	if evt.Kind != EventAskUserRequest {
		t.Fatalf("unexpected kind: %s", evt.Kind)
	}
	if evt.Prompt != "Pick one" {
		t.Fatalf("unexpected prompt: %q", evt.Prompt)
	}
	if !strings.Contains(evt.Input, "1. A") || !strings.Contains(evt.Input, "2. B") {
		t.Fatalf("unexpected options: %q", evt.Input)
	}
}

func TestFromUIEvent_AskUserBatchPrompt(t *testing.T) {
	evt, ok := FromUIEvent(query.UIEvent{
		Kind:     query.UIEventAskUserBatchPrompt,
		ToolName: "ask_user",
		Questions: []tool.AskUserQuestion{
			{Header: "Project", Question: "Project type?"},
			{Header: "Target", Question: "Target env?"},
		},
	})
	if !ok {
		t.Fatalf("expected mapping to succeed")
	}
	if evt.Kind != EventAskUserRequest {
		t.Fatalf("unexpected kind: %s", evt.Kind)
	}
	if evt.Prompt != "2 questions" {
		t.Fatalf("unexpected prompt: %q", evt.Prompt)
	}
	if !strings.Contains(evt.Input, "1. Project") || !strings.Contains(evt.Input, "2. Target") {
		t.Fatalf("unexpected options: %q", evt.Input)
	}
}

func TestTextRenderer_DebugState(t *testing.T) {
	var buf bytes.Buffer
	r := NewTextRenderer(&buf, true)
	r.Emit(Event{
		Kind: EventState,
		Footer: FooterState{
			PermissionMode: "default",
			PlanMode:       false,
			Model:          "claude-sonnet",
			TodoCount:      2,
			MCPCount:       1,
			SkillCount:     0,
			Workspace:      "local",
			SessionID:      "abc",
			CWD:            "Argus",
		},
	})
	out := buf.String()
	if !strings.Contains(out, "state: permission=default") {
		t.Fatalf("expected canonical state line, got %q", out)
	}
}
