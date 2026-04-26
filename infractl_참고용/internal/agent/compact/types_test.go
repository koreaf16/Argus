// Package compact
// File: types_test.go
// Description: TokenState 계산 및 EstimateTokens 단위 테스트
// Responsibility: types.go 의 순수 함수 경계값 검증

package compact

import (
	"testing"

	"github.com/yourorg/infractl/internal/llm"
)

func TestEstimateTokens(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "hello world"},   // 11 chars → 3 tokens
		{Role: llm.RoleAssistant, Content: "ok done!!"}, // 9 chars → 3 tokens
	}
	got := EstimateTokens(msgs)
	want := (11 + 9) / AvgCharsPerToken
	if got != want {
		t.Errorf("EstimateTokens = %d, want %d", got, want)
	}
}

func TestCalculateRemainingTokens(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: string(make([]byte, 30_000))}, // 10_000 tokens
	}
	s := &State{
		Messages:      msgs,
		ContextWindow: 200_000,
		SystemTokens:  2_000,
	}
	got := CalculateRemainingTokens(s)
	want := 200_000 - 10_000 - 2_000 - ReservedOutputTokens
	if got != want {
		t.Errorf("CalculateRemainingTokens = %d, want %d", got, want)
	}
}

func TestCalculateTokenState(t *testing.T) {
	tests := []struct {
		name          string
		contextWindow int
		msgTokens     int
		systemTokens  int
		want          TokenState
	}{
		{"ok", 200_000, 1_000, 0, TokenOK},
		{"warning", 200_000, 200_000 - WarningBufferTokens - ReservedOutputTokens + 1, 0, TokenWarning},
		{"critical", 200_000, 200_000 - AutoCompactBufferTokens - ReservedOutputTokens + 1, 0, TokenCritical},
		{"overflow", 200_000, 200_000 - BlockingBufferTokens - ReservedOutputTokens + 1, 0, TokenOverflow},
		{"zero_context_window", 0, 1_000, 0, TokenOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			content := string(make([]byte, tc.msgTokens*AvgCharsPerToken))
			s := &State{
				Messages:      []llm.Message{{Role: llm.RoleUser, Content: content}},
				ContextWindow: tc.contextWindow,
				SystemTokens:  tc.systemTokens,
			}
			got := CalculateTokenState(s)
			if got != tc.want {
				t.Errorf("CalculateTokenState = %v, want %v", got, tc.want)
			}
		})
	}
}
