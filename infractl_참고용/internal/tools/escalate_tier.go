// Package tools
// File: escalate_tier.go
// Description: general LLM이 reasoning tier 재실행을 요청하는 도구
// Responsibility: escalate_to_reasoning 도구 정의 — 복잡한 분석·다단계 계획 필요 시 호출

package tools

import (
	"context"

	"github.com/yourorg/infractl/internal/executor"
)

// EscalateTierSignal은 escalate_to_reasoning 도구 이름이다.
// loop.go에서 이 이름으로 escalation 신호를 감지한다.
const EscalateTierSignal = "escalate_to_reasoning"

// EscalateToReasoningTool은 general LLM이 현재 턴을 reasoning tier로 재실행 요청하는 도구다.
type EscalateToReasoningTool struct{}

func (t *EscalateToReasoningTool) Name() string { return EscalateTierSignal }

func (t *EscalateToReasoningTool) Description() string {
	return "복잡한 분석·다단계 planning·디버깅이 필요할 때 호출한다. 현재 턴을 reasoning tier로 재실행한다."
}

func (t *EscalateToReasoningTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"reason": map[string]interface{}{
				"type":        "string",
				"description": "reasoning tier가 필요한 이유 (예: 다단계 설치, 장애 원인 분석, 아키텍처 설계)",
			},
		},
		"required": []string{"reason"},
	}
}

func (t *EscalateToReasoningTool) IsReadOnly() bool { return true }
func (t *EscalateToReasoningTool) IsEnabled() bool  { return true }

func (t *EscalateToReasoningTool) Execute(_ context.Context, args map[string]interface{}, _ executor.Executor) (ToolOutcome, error) {
	reason, _ := args["reason"].(string)
	return ToolOutcome{
		Content: "Escalating to reasoning tier: " + reason,
		Success: true,
	}, nil
}
