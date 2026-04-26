// Package compact
// File: stack_test.go
// Description: 5단계 compaction pipeline 통합 단위 테스트
// Responsibility: Apply 호출 시 각 단계 실행, breaker open 시 auto skip 검증

package compact

import (
	"context"
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/llm"
)

// trackingBudget records whether Apply was called.
type trackingBudget struct{ called bool }

func (t *trackingBudget) Apply(s *State, maxChars int) CompactionResult {
	t.called = true
	return CompactionResult{Strategy: "budget"}
}

type trackingMicro struct{ called bool }

func (t *trackingMicro) CompactIfNeeded(s *State, keepRecent int) CompactionResult {
	t.called = true
	return CompactionResult{Strategy: "micro"}
}

type trackingCollapse struct{ applyCalled, drainCalled bool }

func (t *trackingCollapse) ApplyIfNeeded(_ *State, _ TokenState) { t.applyCalled = true }
func (t *trackingCollapse) Drain(_ *State) bool                  { t.drainCalled = true; return false }
func (t *trackingCollapse) HasStaged() bool                      { return false }

type noopSnip struct{}

func (noopSnip) Apply(_ context.Context, _ *State, _ int) CompactionResult {
	return CompactionResult{Strategy: "snip"}
}

type noopAuto struct{}

func (noopAuto) Apply(_ context.Context, _ *State, _ TokenState, _ Breaker) CompactionResult {
	return CompactionResult{Strategy: "auto"}
}

type noopBreaker struct{}

func (noopBreaker) Allow() bool         { return true }
func (noopBreaker) RecordSuccess()      {}
func (noopBreaker) RecordFailure()      {}
func (noopBreaker) Failures() int       { return 0 }

func makeTestStack() (*Stack, *trackingBudget, *trackingMicro, *trackingCollapse) {
	b := &trackingBudget{}
	m := &trackingMicro{}
	col := &trackingCollapse{}
	s := NewStackWith(b, noopSnip{}, m, col, noopAuto{}, noopBreaker{})
	return s, b, m, col
}

func TestStack_ApplyCallsAllStages(t *testing.T) {
	s, b, m, col := makeTestStack()
	state := &State{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "hello"},
		},
		ContextWindow: 200_000,
	}

	s.Apply(context.Background(), state)

	if !b.called {
		t.Error("budget stage not called")
	}
	if !m.called {
		t.Error("micro stage not called")
	}
	if !col.applyCalled {
		t.Error("collapse.ApplyIfNeeded not called")
	}
	if !col.drainCalled {
		t.Error("collapse.Drain not called")
	}
}

func TestStack_SetMildUpdatesAutoCompactor(t *testing.T) {
	called := false
	mild := &mockMild{fn: func() { called = true }}
	auto := NewAuto(nil, mild)

	// Use NewStackWith to bypass budget/snip/micro so they don't compact away the Warning state.
	stack := NewStackWith(&trackingBudget{}, noopSnip{}, &trackingMicro{}, &trackingCollapse{}, auto, noopBreaker{})

	// 17 messages × 31_000 chars = 527_000 chars ≈ 175_666 tokens.
	// remaining = 200_000 - 175_666 - 8_000 = 16_334:
	//   AutoCompactBufferTokens(13k) < 16_334 < WarningBufferTokens(20k) → TokenWarning.
	msgs := make([]llm.Message, 17)
	for i := range msgs {
		msgs[i] = llm.Message{Role: llm.RoleUser, Content: strings.Repeat("x", 31_000)}
	}
	state := &State{Messages: msgs, ContextWindow: 200_000}

	stack.Apply(context.Background(), state)

	if !called {
		t.Error("SetMild: mild.UpdateIfNeeded was not called during Warning state")
	}
}

type mockMild struct{ fn func() }

func (m *mockMild) UpdateIfNeeded(_ context.Context, _ []llm.Message) { m.fn() }
