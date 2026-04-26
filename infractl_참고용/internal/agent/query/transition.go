// Package query
// File: transition.go
// Description: Terminal/Continue 결정 타입 및 TerminalReason 열거
// Responsibility: 루프 종료 판단과 반복 계속 이유를 표현하는 타입 계층
//
// Ported from: claude_cli/src/query/transitions.ts (존재하지 않아 query.ts 내
//              return { reason: '...' } 지점 전수 조사로 재구성)
// 핵심 패턴: TS discriminated union → Go interface + 구체 타입

package query

// TerminalReason 은 query loop 가 종료된 이유를 나타낸다.
// claude_cli: query.ts 내 return { reason: '...' } 에 대응
type TerminalReason string

const (
	// TerminalCompleted: LLM 이 tool_use 없이 텍스트만 반환 → 정상 완료
	TerminalCompleted TerminalReason = "completed"
	// TerminalAbortedStreaming: ctx cancel 또는 사용자 인터럽트로 스트리밍 중단
	TerminalAbortedStreaming TerminalReason = "aborted_streaming"
	// TerminalPromptTooLong: 컨텍스트가 모델 한계를 초과 (PTL / 413)
	TerminalPromptTooLong TerminalReason = "prompt_too_long"
	// TerminalImageError: 이미지 크기/포맷 오류로 중단
	TerminalImageError TerminalReason = "image_error"
	// TerminalModelError: 모델 API 에러 (복구 불가)
	TerminalModelError TerminalReason = "model_error"
	// TerminalMaxTurns: maxTurns 한계 도달
	TerminalMaxTurns TerminalReason = "max_turns"
	// TerminalAbortedTools: 도구 실행 중 abort (sibling abort 최종)
	TerminalAbortedTools TerminalReason = "aborted_tools"
	// TerminalHookStopped: PostToolUse / stop hook 이 중단 신호 발생
	TerminalHookStopped TerminalReason = "hook_stopped"
	// TerminalStopHookPrevented: stop hook 이 계속을 차단
	TerminalStopHookPrevented TerminalReason = "stop_hook_prevented"
	// TerminalBlockingLimit: 동시 실행 도구 차단 한계 도달
	TerminalBlockingLimit TerminalReason = "blocking_limit"
	// TerminalInterrupted: ctx.Done() 으로 인한 외부 중단
	TerminalInterrupted TerminalReason = "interrupted"
)

// ContinueReason 은 loop 가 한 iteration 을 마치고 계속 진행하는 이유를 나타낸다.
// state.transition 필드에 저장되며 첫 iteration 에서는 nil 이다.
// claude_cli: transition: { reason: '...' } 패턴 (query.ts 내 continue 직전)
type ContinueReason string

const (
	// ContinueNextTurn: tool_use 결과 후 정상적인 다음 turn
	ContinueNextTurn ContinueReason = "next_turn"
	// ContinueCollapseDrainRetry: context collapse 후 재시도
	ContinueCollapseDrainRetry ContinueReason = "collapse_drain_retry"
	// ContinueReactiveCompactRetry: reactive compaction 후 재시도
	ContinueReactiveCompactRetry ContinueReason = "reactive_compact_retry"
	// ContinueMaxOutputTokensEscalate: max_output_tokens 에스컬레이션
	ContinueMaxOutputTokensEscalate ContinueReason = "max_output_tokens_escalate"
	// ContinueMaxOutputTokensRecovery: max_output_tokens 복구 재시도
	ContinueMaxOutputTokensRecovery ContinueReason = "max_output_tokens_recovery"
	// ContinueStopHookBlocking: stop hook 이 차단 중 — 결과 대기
	ContinueStopHookBlocking ContinueReason = "stop_hook_blocking"
	// ContinueTokenBudgetContinuation: 토큰 예산 자동 계속
	ContinueTokenBudgetContinuation ContinueReason = "token_budget_continuation"
	// ContinueResearchAuto: research metadata 기반 synthetic auto-continue
	ContinueResearchAuto ContinueReason = "research_auto"
)

// IsTerminal 은 TerminalReason 이 에러 종료인지 여부를 반환한다.
// 정상 완료(completed)와 hook 종료(hook_stopped)는 false 를 반환한다.
func (r TerminalReason) IsError() bool {
	switch r {
	case TerminalCompleted, TerminalHookStopped, TerminalStopHookPrevented,
		TerminalMaxTurns, TerminalInterrupted, TerminalAbortedStreaming,
		TerminalAbortedTools:
		return false
	default:
		return true
	}
}
