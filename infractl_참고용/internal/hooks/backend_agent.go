// Package hooks
// File: backend_agent.go
// Description: Agent hook backend — mini-agent를 실행해 HookOutput 반환
// Responsibility: AgentRunner 주입 시 mini-agent 실행 + 결과 파싱; 미주입 시 ErrAgentNotImplemented

// Ported from: claude_cli/src/utils/hooks/execAgentHook.ts

package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrAgentNotImplemented는 agent backend에 AgentRunner가 주입되지 않았음을 나타낸다.
var ErrAgentNotImplemented = errors.New("agent hook backend: AgentRunner not injected")

// agentBackend는 mini-agent fork를 사용하는 hook backend이다.
type agentBackend struct {
	runner AgentRunner // SetAgentRunner로 주입 — nil이면 ErrAgentNotImplemented 반환
}

// Run은 mini-agent를 실행하고 HookOutput을 반환한다.
// runner가 nil이면 ErrAgentNotImplemented를 반환한다.
func (b *agentBackend) Run(ctx context.Context, def HookDef, input HookInput) (HookOutput, error) {
	if b.runner == nil {
		return HookOutput{}, ErrAgentNotImplemented
	}

	prompt, err := buildAgentPrompt(def, input)
	if err != nil {
		return HookOutput{}, fmt.Errorf("agent hook build prompt: %w", err)
	}

	answer, toolUses, err := b.runner.RunForHook(ctx, prompt)
	if err != nil {
		return HookOutput{}, fmt.Errorf("agent hook run: %w", err)
	}

	return parseAgentAnswer(answer, toolUses), nil
}

// buildAgentPrompt는 HookDef.Agent 템플릿에 HookInput JSON을 치환해 최종 프롬프트를 구성한다.
// $ARGUMENTS 플레이스홀더를 HookInput JSON으로 치환한다.
func buildAgentPrompt(def HookDef, input HookInput) (string, error) {
	jsonBytes, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("marshal hook input: %w", err)
	}
	argStr := string(jsonBytes)

	template := def.Agent
	if template == "" {
		// 기본 지시: allow/deny 결정 요청 (Phase D 스키마)
		template = "다음 훅 이벤트를 분석하고 허용(allow) 또는 거부(deny) 여부를 결정하세요.\n" +
			"반드시 JSON 형식으로 응답하세요: " +
			"{\"decision\": \"allow\"|\"deny\", \"reason\": \"이유\", \"systemMessage\": \"사용자에게 보일 메시지\"}\n\n" +
			"이벤트: $ARGUMENTS"
	}

	prompt := strings.ReplaceAll(template, "$ARGUMENTS", argStr)
	return prompt, nil
}

// parseAgentAnswer는 mini-agent 답변에서 HookOutput을 추출한다.
// JSON {decision, reason, systemMessage} 파싱을 우선하고, 실패 시 텍스트 기반 휴리스틱을 사용한다.
func parseAgentAnswer(answer string, toolUses int) HookOutput {
	// JSON 파싱 시도
	start := strings.Index(answer, "{")
	end := strings.LastIndex(answer, "}")
	if start >= 0 && end > start {
		var out HookOutput
		if err := json.Unmarshal([]byte(answer[start:end+1]), &out); err == nil {
			return out
		}
	}

	// 텍스트 기반 휴리스틱 (JSON 파싱 실패 시 폴백)
	lower := strings.ToLower(answer)
	denied := strings.Contains(lower, "deny") ||
		strings.Contains(lower, "block") ||
		strings.Contains(lower, "reject") ||
		strings.Contains(lower, "거부") ||
		strings.Contains(lower, "차단")

	if denied {
		reason := answer
		if len(reason) > 200 {
			reason = reason[:200] + "..."
		}
		return HookOutput{Decision: DecisionDeny, Reason: reason, SystemMessage: reason}
	}

	// tool 을 사용한 결정이면 결과를 Reason 에 기록
	reason := ""
	if toolUses > 0 && answer != "" {
		reason = answer
		if len(reason) > 200 {
			reason = reason[:200] + "..."
		}
	}
	return HookOutput{Decision: DecisionAllow, Reason: reason}
}
