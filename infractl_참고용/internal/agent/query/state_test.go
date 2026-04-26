// Package query
// File: state_test.go
// Description: QueryState 초기화 및 transition 복사 검증

package query

import (
	"testing"

	"github.com/yourorg/infractl/internal/llm"
)

func TestInitialState(t *testing.T) {
	msgs := []llm.Message{{Role: llm.RoleUser, Content: "hello"}}
	s := initialState(msgs)

	if s.turnCount != 1 {
		t.Errorf("want turnCount=1, got %d", s.turnCount)
	}
	if s.transition != nil {
		t.Error("want transition=nil on first iteration")
	}
	if len(s.messages) != 1 {
		t.Errorf("want 1 message, got %d", len(s.messages))
	}
}

func TestStateWithTransition(t *testing.T) {
	s := initialState(nil)
	reason := ContinueNextTurn
	s2 := s.withTransition(reason)

	if s2.transition == nil {
		t.Fatal("withTransition: expected non-nil transition")
	}
	if *s2.transition != ContinueNextTurn {
		t.Errorf("want ContinueNextTurn, got %q", *s2.transition)
	}
	// 원본 state 는 변경되지 않아야 한다 (값 복사)
	if s.transition != nil {
		t.Error("withTransition must not mutate the original state")
	}
}

func TestStateTransitionDoesNotMutateOriginal(t *testing.T) {
	s := initialState([]llm.Message{{Role: llm.RoleUser, Content: "test"}})
	r1 := ContinueNextTurn
	s.transition = &r1

	r2 := ContinueReactiveCompactRetry
	s2 := s.withTransition(r2)

	if *s.transition != ContinueNextTurn {
		t.Error("original state transition should remain ContinueNextTurn")
	}
	if *s2.transition != ContinueReactiveCompactRetry {
		t.Error("new state should have ContinueReactiveCompactRetry")
	}
}
