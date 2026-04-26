// Package query
// File: transition_test.go
// Description: TerminalReason.IsError() 및 ContinueReason 상수 검증

package query

import "testing"

func TestTerminalReasonIsError(t *testing.T) {
	nonErrors := []TerminalReason{
		TerminalCompleted,
		TerminalHookStopped,
		TerminalStopHookPrevented,
		TerminalMaxTurns,
		TerminalInterrupted,
		TerminalAbortedStreaming,
		TerminalAbortedTools,
	}
	for _, r := range nonErrors {
		if r.IsError() {
			t.Errorf("expected %q to not be an error terminal", r)
		}
	}

	errorReasons := []TerminalReason{
		TerminalPromptTooLong,
		TerminalImageError,
		TerminalModelError,
		TerminalBlockingLimit,
	}
	for _, r := range errorReasons {
		if !r.IsError() {
			t.Errorf("expected %q to be an error terminal", r)
		}
	}
}

func TestContinueReasonValues(t *testing.T) {
	// 모든 ContinueReason 이 비어 있지 않은지 검증
	reasons := []ContinueReason{
		ContinueNextTurn,
		ContinueCollapseDrainRetry,
		ContinueReactiveCompactRetry,
		ContinueMaxOutputTokensEscalate,
		ContinueMaxOutputTokensRecovery,
		ContinueStopHookBlocking,
		ContinueTokenBudgetContinuation,
		ContinueResearchAuto,
	}
	for _, r := range reasons {
		if r == "" {
			t.Error("ContinueReason must not be empty string")
		}
	}
}
