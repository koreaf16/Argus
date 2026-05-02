package presentation

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/koreaf16/argus/internal/query"
	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/state"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
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

func TestBuildFooterState_PopulatesWorkflowFields(t *testing.T) {
	app := state.NewAppState()
	app.SetSessionID("sess-1")
	app.SetTodos("sess-1", []types.TodoItem{
		{Content: "discover", ActiveForm: "Discovering", Status: types.TodoStatusCompleted},
		{Content: "research", ActiveForm: "Researching", Status: types.TodoStatusInProgress},
		{Content: "plan", ActiveForm: "Planning", Status: types.TodoStatusPending},
	})
	app.SetWorkflowCard(&state.WorkflowCard{
		Title:     "Migrate db",
		Category:  state.WorkflowCategoryMigration,
		Phase:     state.WorkflowPhaseResearch,
		StartedAt: time.Now(),
	})

	footer := BuildFooterState(app, "/repo")
	if footer.WorkflowCategory != "migration" {
		t.Fatalf("expected migration category, got %q", footer.WorkflowCategory)
	}
	if footer.WorkflowPhase != "research" {
		t.Fatalf("expected research phase, got %q", footer.WorkflowPhase)
	}
	if footer.WorkflowProgress != "1/3" {
		t.Fatalf("expected 1/3 progress (1 completed of 3 todos), got %q", footer.WorkflowProgress)
	}
	if footer.WorkflowTitle != "Migrate db" {
		t.Fatalf("unexpected workflow title: %q", footer.WorkflowTitle)
	}
}

func TestBuildFooterState_NoWorkflowLeavesFieldsEmpty(t *testing.T) {
	app := state.NewAppState()
	footer := BuildFooterState(app, "/repo")
	if footer.WorkflowPhase != "" || footer.WorkflowCategory != "" {
		t.Fatalf("expected empty workflow fields, got %+v", footer)
	}
}

func TestCanonicalStateLine_IncludesWorkflow(t *testing.T) {
	s := FooterState{
		PermissionMode:   "default",
		Workspace:        "local",
		WorkflowCategory: "migration",
		WorkflowPhase:    "research",
		WorkflowProgress: "1/3",
	}
	line := CanonicalStateLine(s)
	if !strings.Contains(line, "workflow=migration:research/1/3") {
		t.Fatalf("expected workflow segment, got %q", line)
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
