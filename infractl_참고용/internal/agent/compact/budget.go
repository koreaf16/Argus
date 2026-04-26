// Package compact
// File: budget.go
// Description: tool_result 토큰 예산 적용 — 초과 메시지를 절단해 토큰 폭탄 방지
// Responsibility: 가장 큰 tool_result부터 maxChars 이내로 절단

package compact

import (
	"fmt"
	"log/slog"
	"sort"
)

// DefaultBudgetMaxChars는 단일 tool_result 기본 최대 문자 수다.
// Ported from: claude_cli/src/query.ts:379 (applyToolResultBudget 호출부)
const DefaultBudgetMaxChars = 50_000

// Budget은 tool_result 예산 적용 인터페이스다.
// 소비자(stack.go)가 이 패키지에서 정의한다 (Go 관용구).
type Budget interface {
	Apply(s *State, maxChars int) CompactionResult
}

// NewBudget은 Budget 구현체를 반환한다.
func NewBudget() Budget {
	return &budgetApplier{}
}

type budgetApplier struct{}

// Apply는 maxChars 를 초과하는 메시지 Content 를 절단한다.
// Ported from: claude_cli/src/query.ts:379 applyToolResultBudget 호출부
// 핵심 패턴: 가장 큰 메시지부터 절단, truncated 마커 삽입
func (b *budgetApplier) Apply(s *State, maxChars int) CompactionResult {
	if maxChars <= 0 {
		maxChars = DefaultBudgetMaxChars
	}
	pre := len(s.Messages)

	type candidate struct {
		idx  int
		size int
	}
	var cands []candidate
	for i, msg := range s.Messages {
		if len(msg.Content) > maxChars {
			cands = append(cands, candidate{i, len(msg.Content)})
		}
	}
	if len(cands) == 0 {
		return CompactionResult{PreCount: pre, PostCount: pre, Strategy: "budget"}
	}

	sort.Slice(cands, func(i, j int) bool { return cands[i].size > cands[j].size })

	tokensSaved := 0
	for _, c := range cands {
		removed := len(s.Messages[c.idx].Content) - maxChars
		s.Messages[c.idx].Content = fmt.Sprintf(
			"%s\n[truncated: %d chars removed]",
			s.Messages[c.idx].Content[:maxChars],
			removed,
		)
		tokensSaved += removed / AvgCharsPerToken
		slog.Debug("compact budget: truncated",
			"msg_idx", c.idx,
			"original_chars", c.size,
			"removed_chars", removed,
		)
	}

	return CompactionResult{
		PreCount:    pre,
		PostCount:   len(s.Messages),
		Reason:      fmt.Sprintf("%d messages truncated", len(cands)),
		Strategy:    "budget",
		TokensSaved: tokensSaved,
	}
}

// budgetApplier 는 llm.Message 의 Content 만 수정하고 ToolCalls 는 건드리지 않는다.
var _ Budget = (*budgetApplier)(nil)
