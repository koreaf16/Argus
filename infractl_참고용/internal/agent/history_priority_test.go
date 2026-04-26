// Package agent
// File: history_priority_test.go
// Description: trimHistory() 우선순위 기반 제거 검증 테스트
// Responsibility: 텍스트 전용 라운드가 도구 호출 라운드보다 우선 제거됨을 검증

package agent

import (
	"testing"

	"github.com/yourorg/infractl/internal/llm"
)

// makeTextRound는 user+assistant(텍스트 전용) 라운드를 만든다.
func makeTextRound(userText, assistantText string) []llm.Message {
	return []llm.Message{
		{Role: llm.RoleUser, Content: userText},
		{Role: llm.RoleAssistant, Content: assistantText},
	}
}

// makeToolRound는 user+assistant(도구 호출)+tool_result 라운드를 만든다.
func makeToolRound(userText string) []llm.Message {
	return []llm.Message{
		{Role: llm.RoleUser, Content: userText},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "t1", Function: llm.FunctionCall{Name: "shell_exec"}}}},
		{Role: llm.RoleTool, Content: "result"},
	}
}

func TestRoundPriorityScore_TextOnly(t *testing.T) {
	r := apiRound{messages: makeTextRound("hi", "hello")}
	if got := roundPriorityScore(r); got != 0 {
		t.Errorf("text-only score = %d, want 0", got)
	}
}

func TestRoundPriorityScore_WithToolCalls(t *testing.T) {
	r := apiRound{messages: makeToolRound("run ls")}
	if got := roundPriorityScore(r); got != 1 {
		t.Errorf("tool round score = %d, want 1", got)
	}
}

// TestTrimHistory_PrioritizesToolRoundsOverTextRounds verifies that
// text-only rounds are removed before tool-call rounds even if the tool round is older.
func TestTrimHistory_PrioritizesToolRoundsOverTextRounds(t *testing.T) {
	// Layout (2 msgs per round for simplicity):
	//   trimmable: [text text text tool]  (3 text + 1 tool, each 2 msgs = 8 msgs)
	//   preserved: [r r r r]              (4 recent × 2 msgs = 8 msgs)
	// Total = 16, maxHistory = 10 → need to remove 6 msgs = 3 rounds
	// Priority: 3 text rounds removed, tool round survives
	a := &Agent{maxHistory: 10}

	var msgs []llm.Message
	msgs = append(msgs, makeTextRound("textA", "answerA")...)
	msgs = append(msgs, makeTextRound("textB", "answerB")...)
	msgs = append(msgs, makeTextRound("textC", "answerC")...)
	// tool round: user + assistant with tool call (2 msgs)
	msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: "run cmd"})
	msgs = append(msgs, llm.Message{
		Role:      llm.RoleAssistant,
		ToolCalls: []llm.ToolCall{{ID: "t1", Function: llm.FunctionCall{Name: "shell_exec"}}},
	})
	for i := 0; i < preserveRecentRounds; i++ {
		msgs = append(msgs, makeTextRound("recent", "recent answer")...)
	}
	a.history = msgs

	a.trimHistory()

	if len(a.history) > a.maxHistory {
		t.Errorf("history len = %d, want <= %d", len(a.history), a.maxHistory)
	}

	// The tool round assistant message should still be in history
	foundTool := false
	for _, msg := range a.history {
		if msg.Role == llm.RoleAssistant && len(msg.ToolCalls) > 0 {
			foundTool = true
			break
		}
	}
	if !foundTool {
		t.Error("tool round was incorrectly removed; should have survived over text-only rounds")
	}
}

func TestTrimHistory_NoopWhenUnderLimit(t *testing.T) {
	a := &Agent{maxHistory: 100}
	msgs := makeTextRound("hi", "hello")
	a.history = msgs

	a.trimHistory()
	if len(a.history) != len(msgs) {
		t.Errorf("history changed when under limit: %d -> %d", len(msgs), len(a.history))
	}
}

func TestTrimHistory_PreservesRecentRounds(t *testing.T) {
	// 8 rounds × 2 msgs = 16 total. maxHistory=12 → need to remove 4 msgs = 2 rounds.
	// The last preserveRecentRounds rounds must not be touched.
	a := &Agent{maxHistory: 12}

	var msgs []llm.Message
	for i := 0; i < 8; i++ {
		msgs = append(msgs, makeTextRound("q", "a")...)
	}
	a.history = msgs

	a.trimHistory()

	if len(a.history) > a.maxHistory {
		t.Errorf("history len = %d, want <= %d", len(a.history), a.maxHistory)
	}
	// At least the preserved recent rounds must remain
	minExpected := preserveRecentRounds * 2
	if len(a.history) < minExpected {
		t.Errorf("expected at least %d msgs (recent rounds), got %d", minExpected, len(a.history))
	}
}
