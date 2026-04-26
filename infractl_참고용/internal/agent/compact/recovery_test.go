// Package compact
// File: recovery_test.go
// Description: 에러 복구 분기 단위 테스트
// Responsibility: 각 에러 타입 → RecoveryAction 매핑, flag 전환 검증

package compact

import (
	"context"
	"errors"
	"testing"
)

type stubCollapse struct {
	hasStaged bool
	drained   bool
}

func (s *stubCollapse) ApplyIfNeeded(_ *State, _ TokenState) {}
func (s *stubCollapse) Drain(_ *State) bool {
	if s.hasStaged {
		s.drained = true
		return true
	}
	return false
}
func (s *stubCollapse) HasStaged() bool { return s.hasStaged }

type stubReactive struct {
	called bool
}

func (r *stubReactive) TryCompact(_ context.Context, s *State) (CompactionResult, error) {
	r.called = true
	s.HasAttemptedReactiveCompact = true
	return CompactionResult{}, nil
}

func newTestRecovery(col Collapse, react Reactive) *Recovery {
	return NewRecovery(col, react, nil)
}

func TestRecovery_PTLDrainsCollapse(t *testing.T) {
	col := &stubCollapse{hasStaged: true}
	r := newTestRecovery(col, nil)

	action, err := r.Handle(context.Background(),&State{}, ptlErr())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if action != ActionDrainCollapse {
		t.Errorf("expected ActionDrainCollapse, got %v", action)
	}
	if !col.drained {
		t.Error("Drain should have been called")
	}
}

func TestRecovery_PTLReactiveWhenNoneStaged(t *testing.T) {
	react := &stubReactive{}
	r := newTestRecovery(&stubCollapse{hasStaged: false}, react)
	s := &State{}

	action, err := r.Handle(context.Background(),s, ptlErr())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if action != ActionReactiveCompact {
		t.Errorf("expected ActionReactiveCompact, got %v", action)
	}
	if !s.HasAttemptedReactiveCompact {
		t.Error("HasAttemptedReactiveCompact should be set")
	}
}

func TestRecovery_PTLSurfaceWhenBothExhausted(t *testing.T) {
	r := newTestRecovery(&stubCollapse{hasStaged: false}, nil)
	s := &State{HasAttemptedReactiveCompact: true}

	action, _ := r.Handle(context.Background(),s, ptlErr())
	if action != ActionSurfaceError {
		t.Errorf("expected ActionSurfaceError, got %v", action)
	}
}

func TestRecovery_MaxOutputEscalatesFirst(t *testing.T) {
	r := newTestRecovery(nil, nil)
	s := &State{}

	action, err := r.Handle(context.Background(),s, maxOutputErr())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if action != ActionRetryMaxTokens {
		t.Errorf("expected ActionRetryMaxTokens, got %v", action)
	}
	if s.MaxOutputTokensRecoveryCount != 1 {
		t.Errorf("count should be 1, got %d", s.MaxOutputTokensRecoveryCount)
	}
	if s.MaxOutputTokensOverride == nil || *s.MaxOutputTokensOverride != MaxTokensEscalation {
		t.Errorf("MaxOutputTokensOverride should be %d", MaxTokensEscalation)
	}
}

func TestRecovery_MaxOutputMultiTurnSecondTime(t *testing.T) {
	r := newTestRecovery(nil, nil)
	s := &State{MaxOutputTokensRecoveryCount: 1}

	action, _ := r.Handle(context.Background(),s, maxOutputErr())
	if action != ActionMultiTurnRecovery {
		t.Errorf("expected ActionMultiTurnRecovery, got %v", action)
	}
}

func TestRecovery_MediaStrip(t *testing.T) {
	r := newTestRecovery(nil, nil)
	action, _ := r.Handle(context.Background(),&State{}, errors.New("invalid image: too large"))
	if action != ActionStripMedia {
		t.Errorf("expected ActionStripMedia, got %v", action)
	}
}

func TestRecovery_FallbackStripsSignatures(t *testing.T) {
	r := newTestRecovery(nil, nil)
	action, _ := r.Handle(context.Background(),&State{}, errors.New("overloaded"))
	if action != ActionStripSignaturesAndRetry {
		t.Errorf("expected ActionStripSignaturesAndRetry, got %v", action)
	}
}

func TestRecovery_UnknownSurfaces(t *testing.T) {
	r := newTestRecovery(nil, nil)
	action, _ := r.Handle(context.Background(),&State{}, errors.New("some unknown error"))
	if action != ActionSurfaceError {
		t.Errorf("expected ActionSurfaceError, got %v", action)
	}
}

func ptlErr() error       { return errors.New("too many tokens in context window") }
func maxOutputErr() error { return errors.New("stop_reason: max_tokens reached") }
