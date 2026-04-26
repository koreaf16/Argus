// Package compact
// File: stack.go
// Description: 5단계 compaction stack — budget→snip→micro→collapse→auto 순서 적용
// Responsibility: Stack 조립, Apply 순차 실행, 각 단계 slog 구조화 로그
//
// Ported from: claude_cli/src/query.ts:396-468 (5단계 순차 compaction 호출)
// 핵심 패턴: budget→snip→micro→collapse→auto 순서, 각 단계 독립 실행

package compact

import (
	"context"
	"log/slog"

	"github.com/yourorg/infractl/internal/llm"
)

// Stack은 5단계 compaction 파이프라인이다.
type Stack struct {
	budget   Budget
	snip     Snip
	micro    Micro
	collapse Collapse
	auto     Auto
	breaker  Breaker
}

// NewStack은 기본 설정으로 Stack을 조립한다.
func NewStack(client llm.Client, mild MildCompactor) *Stack {
	return &Stack{
		budget:   NewBudget(),
		snip:     NewSnip(client),
		micro:    NewMicro(),
		collapse: NewCollapse(),
		auto:     NewAuto(client, mild),
		breaker:  NewBreaker(),
	}
}

// NewStackWith는 외부에서 전략을 주입해 Stack을 조립한다. 테스트에서 사용한다.
func NewStackWith(budget Budget, snip Snip, micro Micro, collapse Collapse, auto Auto, breaker Breaker) *Stack {
	return &Stack{
		budget:   budget,
		snip:     snip,
		micro:    micro,
		collapse: collapse,
		auto:     auto,
		breaker:  breaker,
	}
}

// Apply는 5단계 compaction 파이프라인을 순서대로 실행한다.
// Ported from: claude_cli/src/query.ts:396-468
// 단계: budget → snip → micro → collapse → auto
func (s *Stack) Apply(ctx context.Context, state *State) {
	preTotalTokens := EstimateTokens(state.Messages)

	// 1. budget: 단일 tool_result 절단
	budgetResult := s.budget.Apply(state, DefaultBudgetMaxChars)
	logStep("budget", budgetResult)

	// 2. snip: 오래된 turn 전체 요약
	snipResult := s.snip.Apply(ctx, state, DefaultKeepRecentTurns)
	logStep("snip", snipResult)

	// 3. micro: 오래된 tool_result 메시지 truncation
	microResult := s.micro.CompactIfNeeded(state, DefaultKeepRecentMessages)
	logStep("micro", microResult)

	// 4. collapse: staged queue 적재 + 이전 turn에서 적재된 항목 drain
	tokenState := CalculateTokenState(state)
	s.collapse.ApplyIfNeeded(state, tokenState)
	if s.collapse.Drain(state) {
		slog.Debug("compact stack: collapse drain applied")
	}

	// 5. auto: proactive compaction (tokenState 재계산 — 이전 단계 효과 반영)
	tokenState = CalculateTokenState(state)
	autoResult := s.auto.Apply(ctx, state, tokenState, s.breaker)
	logStep("auto", autoResult)

	postTotalTokens := EstimateTokens(state.Messages)
	totalSaved := preTotalTokens - postTotalTokens
	if totalSaved > 0 {
		slog.Info("compact stack: apply complete",
			"pre_tokens", preTotalTokens,
			"post_tokens", postTotalTokens,
			"total_saved", totalSaved,
			"token_state", tokenState,
		)
	}
}

// Collapse는 내부 Collapse 인스턴스를 반환한다. Recovery에서 HasStaged 호출용.
func (s *Stack) Collapse() Collapse {
	return s.collapse
}

// Reactive는 새 Reactive 인스턴스를 생성해 반환한다.
// recovery.go 에서 compact client 가 필요할 때 사용한다.
func (s *Stack) NewReactive(client llm.Client) Reactive {
	return NewReactive(client)
}

// SetMild는 auto compaction 의 mild 전략 실행자를 교체한다.
// agent.SetSessionSummary 주입 시 호출하여 SessionSummaryManager 를 연결한다.
func (s *Stack) SetMild(m MildCompactor) {
	if ac, ok := s.auto.(*autoCompactor); ok {
		ac.mild = m
	}
}

func logStep(name string, r CompactionResult) {
	if r.TokensSaved > 0 || r.Reason != "" {
		slog.Debug("compact stack: step",
			"step", name,
			"strategy", r.Strategy,
			"pre_count", r.PreCount,
			"post_count", r.PostCount,
			"tokens_saved", r.TokensSaved,
			"reason", r.Reason,
		)
	}
}
