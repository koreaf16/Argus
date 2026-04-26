// Package agent
// File: plan_state.go
// Description: 다단계 작업 계획의 상태머신 — propose_action의 plan 항목을 단계별로 추적
// Responsibility: PlanStep 상태 전이, 현재 단계 조회, 요약 텍스트 생성

package agent

import (
	"fmt"
	"strings"
	"time"
)

// PlanStepStatus는 계획 단계의 실행 상태이다.
type PlanStepStatus string

const (
	PlanStepPending    PlanStepStatus = "pending"
	PlanStepInProgress PlanStepStatus = "in_progress"
	PlanStepCompleted  PlanStepStatus = "completed"
	PlanStepFailed     PlanStepStatus = "failed"
	PlanStepSkipped    PlanStepStatus = "skipped"
)

// PlanStep은 계획의 단일 실행 단계이다.
type PlanStep struct {
	Index       int
	Description string
	Command     string
	Status      PlanStepStatus
	StartedAt   *time.Time
	EndedAt     *time.Time
	ErrorNote   string
}

// PlanState는 propose_action으로 시작된 다단계 작업 계획의 런타임 상태이다.
type PlanState struct {
	Steps        []PlanStep
	CurrentIndex int
	CreatedAt    time.Time
}

// NewPlanState는 문자열 단계 목록으로 PlanState를 초기화한다.
func NewPlanState(steps []string) *PlanState {
	ps := &PlanState{
		Steps:     make([]PlanStep, 0, len(steps)),
		CreatedAt: time.Now(),
	}
	for i, s := range steps {
		ps.Steps = append(ps.Steps, PlanStep{
			Index:       i,
			Description: s,
			Status:      PlanStepPending,
		})
	}
	return ps
}

// Current는 현재 실행 대상 단계를 반환한다. 모든 단계가 완료되면 nil을 반환한다.
func (p *PlanState) Current() *PlanStep {
	for i := range p.Steps {
		if p.Steps[i].Status == PlanStepPending || p.Steps[i].Status == PlanStepInProgress {
			return &p.Steps[i]
		}
	}
	return nil
}

// Advance는 현재 단계를 주어진 상태로 전이시키고 다음 단계로 진행한다.
func (p *PlanState) Advance(status PlanStepStatus, errNote string) {
	now := time.Now()
	for i := range p.Steps {
		if p.Steps[i].Status == PlanStepPending || p.Steps[i].Status == PlanStepInProgress {
			p.Steps[i].Status = status
			p.Steps[i].EndedAt = &now
			if errNote != "" {
				p.Steps[i].ErrorNote = errNote
			}
			p.CurrentIndex = i + 1
			return
		}
	}
}

// IsComplete는 모든 단계가 종료 상태(completed/failed/skipped)인지 확인한다.
func (p *PlanState) IsComplete() bool {
	for _, s := range p.Steps {
		if s.Status == PlanStepPending || s.Status == PlanStepInProgress {
			return false
		}
	}
	return true
}

// FormatSummary는 현재 계획 진행 상황을 사람이 읽기 좋은 텍스트로 반환한다.
func (p *PlanState) FormatSummary() string {
	var sb strings.Builder
	sb.WriteString("**작업 계획 진행 상황**\n")
	for _, s := range p.Steps {
		var icon string
		switch s.Status {
		case PlanStepCompleted:
			icon = "✓"
		case PlanStepFailed:
			icon = "✗"
		case PlanStepSkipped:
			icon = "—"
		case PlanStepInProgress:
			icon = "▶"
		default:
			icon = "○"
		}
		sb.WriteString(fmt.Sprintf("%s %d. %s\n", icon, s.Index+1, s.Description))
		if s.ErrorNote != "" {
			sb.WriteString(fmt.Sprintf("   오류: %s\n", s.ErrorNote))
		}
	}
	return sb.String()
}
