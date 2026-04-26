// Package taskctx
// File: proposal.go
// Description: LLM 제안을 사용자 확인 받기 전 보관하는 PendingProposal
// Responsibility: 제안 상태 관리, 사용자 입력 의도 분류(확인/취소/수정), 만료 판정

package taskctx

import (
	"maps"
	"strings"
	"time"
)

// TaskContextDraft 는 DeclareRequest 와 유사하지만 LLM이 채우는 제안용이다.
type TaskContextDraft struct {
	Title         string
	Kind          TaskKind
	TargetServer  string
	TargetAccount string
	WorkingDir    string
	Forbidden     []string
	Preconditions []string
	PlanSteps     []string
	KindFields    map[string]string
}

// ToDeclareRequest 는 확정 시 DeclareRequest 로 변환한다.
func (d TaskContextDraft) ToDeclareRequest() DeclareRequest {
	req := DeclareRequest{
		Title:         d.Title,
		Kind:          d.Kind,
		TargetServer:  d.TargetServer,
		TargetAccount: d.TargetAccount,
		WorkingDir:    d.WorkingDir,
	}
	if len(d.Forbidden) > 0 {
		req.Forbidden = make([]string, len(d.Forbidden))
		copy(req.Forbidden, d.Forbidden)
	}
	if len(d.Preconditions) > 0 {
		req.Preconditions = make([]string, len(d.Preconditions))
		copy(req.Preconditions, d.Preconditions)
	}
	if len(d.PlanSteps) > 0 {
		req.PlanSteps = make([]string, len(d.PlanSteps))
		copy(req.PlanSteps, d.PlanSteps)
	}
	if d.KindFields != nil {
		req.KindFields = make(map[string]string, len(d.KindFields))
		maps.Copy(req.KindFields, d.KindFields)
	}
	return req
}

// PendingProposal 은 LLM 제안을 사용자 확인 받기 전 보관한다.
type PendingProposal struct {
	ID           string
	Draft        TaskContextDraft
	RiskAnalysis string
	RollbackPlan []string
	Reasoning    string
	CreatedAt    time.Time
	TurnIndex    int
}

// confirmationKeywords 는 확인을 의미하는 입력 패턴이다.
var confirmationKeywords = []string{
	"yes", "ok", "okay", "진행", "좋아", "좋아요", "확인", "승인", "동의", "맞아", "그래",
}

// cancelKeywords 는 취소를 의미하는 입력 패턴이다.
var cancelKeywords = []string{
	"취소", "cancel", "no", "아니", "아니요", "안해", "안돼", "그만", "중단",
}

// IsConfirmation 은 사용자 입력이 "yes"/"ok"/"진행"/"좋아" 계열인지 판단한다.
func (p *PendingProposal) IsConfirmation(input string) bool {
	normalized := strings.ToLower(strings.TrimSpace(input))
	for _, kw := range confirmationKeywords {
		if normalized == kw || strings.HasPrefix(normalized, kw) {
			return true
		}
	}
	return false
}

// IsCancel 은 "취소"/"cancel"/"no"/"아니" 계열인지 판단한다.
func (p *PendingProposal) IsCancel(input string) bool {
	normalized := strings.ToLower(strings.TrimSpace(input))
	for _, kw := range cancelKeywords {
		if normalized == kw || strings.HasPrefix(normalized, kw) {
			return true
		}
	}
	return false
}

// IsRevision 은 취소/확정이 아닌 자연어(수정 지시)로 판정한다.
// 반환: (hint, true) — hint 는 수정 방향 텍스트.
func (p *PendingProposal) IsRevision(input string) (string, bool) {
	if p.IsConfirmation(input) || p.IsCancel(input) {
		return "", false
	}
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}

// IsStale 은 TurnIndex 차이 3 이상 또는 5분 초과 시 true 를 반환한다.
func (p *PendingProposal) IsStale(currentTurn int, now time.Time) bool {
	if currentTurn-p.TurnIndex >= 3 {
		return true
	}
	if now.Sub(p.CreatedAt) > 5*time.Minute {
		return true
	}
	return false
}

// Summary 는 한 줄 표시 "[제안 대기] <Title>" 형식의 문자열을 반환한다.
func (p *PendingProposal) Summary() string {
	return "[제안 대기] " + p.Draft.Title
}
