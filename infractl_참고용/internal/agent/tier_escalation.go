// Package agent
// File: tier_escalation.go
// Description: general tier → reasoning tier mid-turn 재실행 지원
// Responsibility: escalation sentinel 정의, ToolDef 필터링, re-invoke 헬퍼

package agent

import (
	"errors"

	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/tools"
)

// errEscalateToReasoning은 general LLM이 escalate_to_reasoning을 호출했을 때
// consumeEngineEvents가 반환하는 sentinel 에러다.
// Run()에서 이를 잡아 reasoning tier로 재호출한다.
var errEscalateToReasoning = errors.New("escalate_to_reasoning requested")

// isEscalationRequest는 에러가 tier escalation 신호인지 판별한다.
func isEscalationRequest(err error) bool {
	return errors.Is(err, errEscalateToReasoning)
}

// filterToolDef는 ToolDef 슬라이스에서 특정 이름의 도구를 제거한 복사본을 반환한다.
func filterToolDef(defs []llm.ToolDef, exclude string) []llm.ToolDef {
	out := make([]llm.ToolDef, 0, len(defs))
	for _, d := range defs {
		if d.Function.Name != exclude {
			out = append(out, d)
		}
	}
	return out
}

// filterAllowedTools는 map[string]bool에서 특정 도구 이름을 제거한 복사본을 반환한다.
func filterAllowedTools(allowed map[string]bool, exclude string) map[string]bool {
	out := make(map[string]bool, len(allowed))
	for k, v := range allowed {
		if k != exclude {
			out[k] = v
		}
	}
	return out
}

// popLastAssistantMessage는 history 슬라이스의 마지막 assistant 메시지를 제거한다.
// escalation 재실행 전 불완전한 general 응답을 history에서 정리한다.
func popLastAssistantMessage(history []llm.Message) []llm.Message {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == llm.RoleAssistant {
			return append(history[:i:i], history[i+1:]...)
		}
	}
	return history
}

// escalationToolDef는 escalate_to_reasoning 도구를 프롬프트에서 제외하기 위한 이름이다.
var escalationToolName = tools.EscalateTierSignal
