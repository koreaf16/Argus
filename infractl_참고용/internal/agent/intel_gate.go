// Package agent
// File: intel_gate.go
// Description: Intel 서브에이전트 실행 전 skip 조건 판정 로직
// Responsibility: classification 결과를 보고 intel 사전 조사가 불필요한 케이스를 걸러낸다

package agent

// shouldSkipIntel은 Intel 서브에이전트 실행을 건너뛸지 여부를 판정한다.
// skip=true면 reason에 skip 이유가 담긴다.
func shouldSkipIntel(isReasoning bool, taskType string, pendingActive bool, isShortFollowUp bool) (skip bool, reason string) {
	switch {
	case pendingActive:
		return true, "pending_action_active"
	case isShortFollowUp:
		return true, "short_follow_up"
	case taskType == "":
		return true, "task_type_unclassified"
	case !isReasoning:
		return true, "general_tier"
	}
	return false, ""
}
