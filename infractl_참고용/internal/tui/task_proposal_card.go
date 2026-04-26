// Package tui
// File: task_proposal_card.go
// Description: LLM이 제안한 작업 환경 카드 렌더링 — 사용자 확인 대기 화면
// Responsibility: ProposalCardData를 받아 제안 카드 텍스트 렌더링

package tui

import (
	"fmt"
	"strings"
)

// ProposalCardState represents the confirmation state of a proposal card.
type ProposalCardState int

const (
	ProposalCardPending  ProposalCardState = iota // Waiting for user response
	ProposalCardApproved                          // User confirmed
	ProposalCardRejected                          // User cancelled
	ProposalCardRevising                          // User requested revision
)

// ProposalCardData holds the data to render in a proposal card.
type ProposalCardData struct {
	Title         string
	Kind          string
	TargetServer  string
	TargetAccount string
	WorkingDir    string
	PlanSteps     []string
	Forbidden     []string
	RollbackPlan  []string
	RiskAnalysis  string
	Reasoning     string
	KindFields    map[string]string
	State         ProposalCardState
}

// RenderTaskProposalCard returns a formatted proposal card string for display.
// state controls outer border color: Pending=indigo, Approved=green, Rejected=red.
func RenderTaskProposalCard(d ProposalCardData, width int) string {
	if width <= 0 {
		width = 80
	}
	innerWidth := width - 6 // account for borders and padding

	// Header line
	stateIcon := "◆"
	stateLabel := "제안 대기"
	outerStyle := StyleTaskProposalOuter
	switch d.State {
	case ProposalCardApproved:
		stateIcon = "✓"
		stateLabel = "확정"
		outerStyle = StyleTaskProposalOK
	case ProposalCardRejected:
		stateIcon = "✗"
		stateLabel = "취소"
		outerStyle = StyleTaskProposalDeny
	case ProposalCardRevising:
		stateIcon = "↺"
		stateLabel = "수정 중"
	}

	title := StyleTaskTitle.Render(fmt.Sprintf("%s %s", stateIcon, d.Title))
	kindBadge := StyleTaskDim.Render("[" + d.Kind + "]")
	header := title + "  " + kindBadge + "  " + StyleTaskDim.Render(stateLabel)

	// Body
	var body strings.Builder
	body.WriteString(header + "\n")
	fmt.Fprintf(&body, "  서버: %s  계정: %s  디렉토리: %s\n",
		nvl(d.TargetServer, "-"), nvl(d.TargetAccount, "-"), nvl(d.WorkingDir, "-"))

	// Kind-specific fields
	if len(d.KindFields) > 0 {
		body.WriteString("\n  [" + d.Kind + " 세부]\n")
		for k, v := range d.KindFields {
			fmt.Fprintf(&body, "  · %s: %s\n", k, v)
		}
	}

	// Plan steps
	if len(d.PlanSteps) > 0 {
		body.WriteString("\n  [단계]\n")
		for i, step := range d.PlanSteps {
			fmt.Fprintf(&body, "  %d. %s\n", i+1, step)
		}
	}

	// Forbidden
	if len(d.Forbidden) > 0 {
		body.WriteString("\n  [금지] " + strings.Join(d.Forbidden, ", ") + "\n")
	}

	// Rollback
	if len(d.RollbackPlan) > 0 {
		body.WriteString("\n  [롤백]\n")
		for i, step := range d.RollbackPlan {
			fmt.Fprintf(&body, "  %d. %s\n", i+1, step)
		}
	}

	// Risk + reasoning
	if d.RiskAnalysis != "" {
		body.WriteString("\n  " + StyleTaskDim.Render("위험: "+truncateCard(d.RiskAnalysis, innerWidth-8)) + "\n")
	}
	if d.Reasoning != "" {
		body.WriteString("  " + StyleTaskDim.Render("이유: "+truncateCard(d.Reasoning, innerWidth-8)) + "\n")
	}

	// Footer hint (only when pending)
	if d.State == ProposalCardPending {
		body.WriteString("\n  " + StyleTaskDim.Render("`yes` 진행 · 자연어로 수정 지시 · `취소` 거부") + "\n")
	}

	return outerStyle.Width(width - 2).Render(body.String())
}

// nvl returns fallback when s is blank.
func nvl(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

// truncateCard truncates s to maxLen characters, appending "..." when trimmed.
func truncateCard(s string, maxLen int) string {
	if len(s) <= maxLen || maxLen <= 3 {
		return s
	}
	return s[:maxLen-3] + "..."
}
