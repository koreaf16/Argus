// Package compact
// File: budget_test.go
// Description: budget compaction (tool_result 절단) 단위 테스트
// Responsibility: Apply 후 메시지 길이 경계 검증

package compact

import (
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/llm"
)

func TestBudgetApply_TruncatesOversizedToolResult(t *testing.T) {
	large := strings.Repeat("x", DefaultBudgetMaxChars+1_000)
	s := &State{
		Messages: []llm.Message{
			{Role: llm.RoleTool, ToolCallID: "t1", Content: large},
			{Role: llm.RoleUser, Content: "ok"},
		},
	}
	b := NewBudget()
	r := b.Apply(s, DefaultBudgetMaxChars)

	if len(s.Messages[0].Content) >= DefaultBudgetMaxChars+1_000 {
		t.Errorf("oversized message not truncated")
	}
	if !strings.Contains(s.Messages[0].Content, "[truncated:") {
		t.Errorf("truncation marker missing")
	}
	if r.TokensSaved <= 0 {
		t.Errorf("expected positive TokensSaved, got %d", r.TokensSaved)
	}
}

func TestBudgetApply_LeavesSmallMessagesUnchanged(t *testing.T) {
	s := &State{
		Messages: []llm.Message{
			{Role: llm.RoleTool, ToolCallID: "t1", Content: "small output"},
			{Role: llm.RoleUser, Content: "ask"},
		},
	}
	b := NewBudget()
	r := b.Apply(s, DefaultBudgetMaxChars)

	if s.Messages[0].Content != "small output" {
		t.Errorf("small message was modified: %q", s.Messages[0].Content)
	}
	if r.TokensSaved != 0 {
		t.Errorf("expected zero TokensSaved, got %d", r.TokensSaved)
	}
}
