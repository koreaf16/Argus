// Package compact
// File: micro_test.go
// Description: message-level micro compaction 단위 테스트
// Responsibility: keepRecent 경계, tool_result 선택적 절단, 최근 메시지 보존 검증

package compact

import (
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/llm"
)

func TestMicro_NoopWhenUnderKeepRecent(t *testing.T) {
	s := &State{
		Messages: []llm.Message{
			{Role: llm.RoleTool, ToolCallID: "t1", Content: strings.Repeat("x", 500)},
			{Role: llm.RoleUser, Content: "ask"},
		},
	}
	m := NewMicro()
	r := m.CompactIfNeeded(s, DefaultKeepRecentMessages)

	if r.TokensSaved != 0 {
		t.Errorf("expected noop, got TokensSaved=%d", r.TokensSaved)
	}
	if len(s.Messages[0].Content) != 500 {
		t.Error("message should not be modified below keepRecent threshold")
	}
}

func TestMicro_TruncatesOlderToolResults(t *testing.T) {
	msgs := make([]llm.Message, DefaultKeepRecentMessages+3)
	for i := range msgs {
		msgs[i] = llm.Message{
			Role:       llm.RoleTool,
			ToolCallID: "id",
			Content:    strings.Repeat("z", 500),
		}
	}
	s := &State{Messages: msgs}

	m := NewMicro()
	r := m.CompactIfNeeded(s, DefaultKeepRecentMessages)

	if r.TokensSaved <= 0 {
		t.Errorf("expected positive TokensSaved, got %d", r.TokensSaved)
	}

	// Recent keepRecent messages must be untouched
	for i := len(msgs) - DefaultKeepRecentMessages; i < len(msgs); i++ {
		if len(s.Messages[i].Content) != 500 {
			t.Errorf("recent message[%d] should not be modified", i)
		}
	}
}

func TestMicro_PreservesUserMessages(t *testing.T) {
	msgs := make([]llm.Message, DefaultKeepRecentMessages+3)
	for i := range msgs {
		msgs[i] = llm.Message{
			Role:    llm.RoleUser,
			Content: strings.Repeat("u", 500),
		}
	}
	s := &State{Messages: msgs}

	m := NewMicro()
	m.CompactIfNeeded(s, DefaultKeepRecentMessages)

	for i, msg := range s.Messages {
		if len(msg.Content) != 500 {
			t.Errorf("user message[%d] should not be modified", i)
		}
	}
}
