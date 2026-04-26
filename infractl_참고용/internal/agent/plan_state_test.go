// Package agent
// File: plan_state_test.go
// Description: PlanState 상태머신 단위 테스트
// Responsibility: 단계 전이, 완료 감지, 요약 텍스트 생성 검증

package agent

import (
	"strings"
	"testing"
)

func TestNewPlanState_InitializesPending(t *testing.T) {
	steps := []string{"단계1", "단계2", "단계3"}
	ps := NewPlanState(steps)

	if len(ps.Steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(ps.Steps))
	}
	for i, s := range ps.Steps {
		if s.Status != PlanStepPending {
			t.Errorf("step[%d].Status = %s, want pending", i, s.Status)
		}
		if s.Index != i {
			t.Errorf("step[%d].Index = %d, want %d", i, s.Index, i)
		}
	}
}

func TestPlanState_Current_ReturnsFirstPending(t *testing.T) {
	ps := NewPlanState([]string{"A", "B", "C"})

	cur := ps.Current()
	if cur == nil {
		t.Fatal("expected non-nil current")
	}
	if cur.Description != "A" {
		t.Errorf("first current = %q, want A", cur.Description)
	}
}

func TestPlanState_Advance_MovesToNext(t *testing.T) {
	ps := NewPlanState([]string{"A", "B", "C"})

	ps.Advance(PlanStepCompleted, "")
	cur := ps.Current()
	if cur == nil || cur.Description != "B" {
		t.Errorf("after advance, current = %v", cur)
	}
	if ps.Steps[0].Status != PlanStepCompleted {
		t.Errorf("step[0] should be completed, got %s", ps.Steps[0].Status)
	}
}

func TestPlanState_IsComplete(t *testing.T) {
	ps := NewPlanState([]string{"A", "B"})

	if ps.IsComplete() {
		t.Error("should not be complete initially")
	}

	ps.Advance(PlanStepCompleted, "")
	ps.Advance(PlanStepCompleted, "")

	if !ps.IsComplete() {
		t.Error("should be complete after all steps advanced")
	}
}

func TestPlanState_Advance_WithError(t *testing.T) {
	ps := NewPlanState([]string{"A", "B"})

	ps.Advance(PlanStepFailed, "permission denied")

	if ps.Steps[0].Status != PlanStepFailed {
		t.Errorf("step[0].Status = %s, want failed", ps.Steps[0].Status)
	}
	if ps.Steps[0].ErrorNote != "permission denied" {
		t.Errorf("ErrorNote = %q", ps.Steps[0].ErrorNote)
	}
}

func TestPlanState_FormatSummary_ContainsSteps(t *testing.T) {
	ps := NewPlanState([]string{"DB Home 압축 해제", "OPatch 교체", "Installer 실행"})
	ps.Advance(PlanStepCompleted, "")

	summary := ps.FormatSummary()
	if !strings.Contains(summary, "DB Home 압축 해제") {
		t.Error("summary missing step 1")
	}
	if !strings.Contains(summary, "OPatch 교체") {
		t.Error("summary missing step 2")
	}
}

func TestPlanState_CurrentNilWhenAllDone(t *testing.T) {
	ps := NewPlanState([]string{"A"})
	ps.Advance(PlanStepSkipped, "")

	if ps.Current() != nil {
		t.Error("Current should be nil when all steps are done")
	}
}
