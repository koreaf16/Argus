// Package hooks
// File: backend_prompt.go
// Description: LLM prompt hook backend 구현
// Responsibility: HookDef.Prompt 를 Fast tier LLM 으로 실행하여 HookOutput 반환

// Ported from: claude_cli/src/utils/hooks/execPromptHook.ts:21-90
// 핵심 패턴: $ARGUMENTS 플레이스홀더 치환 → Fast/Reasoning tier LLM 호출 → JSON 응답 파싱.

package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/hooks/builtins"
	"github.com/yourorg/infractl/internal/llm"
)

// builtinPrefix 는 hooks.yaml 의 prompt 필드에서 내장 템플릿을 참조하는 접두어다.
// 예: "$BUILTIN:shell_validator" → internal/hooks/builtins/agent/shell_validator.md 내용으로 치환.
const builtinPrefix = "$BUILTIN:"

// promptBackend 는 LLM 을 사용하는 hook backend 이다.
type promptBackend struct {
	registry *llm.Registry
}

// Run 은 HookDef.Prompt 를 LLM 에 전달해 HookOutput JSON 을 반환한다.
func (b *promptBackend) Run(ctx context.Context, def HookDef, input HookInput) (HookOutput, error) {
	if def.Prompt == "" {
		return HookOutput{}, fmt.Errorf("prompt hook: empty prompt")
	}

	// $BUILTIN:<name> 접두어가 있으면 내장 프롬프트로 치환
	promptTemplate := def.Prompt
	if name, isBuiltin := strings.CutPrefix(promptTemplate, builtinPrefix); isBuiltin {
		tmpl, found := builtins.LookupPrompt(name)
		if !found {
			return HookOutput{}, fmt.Errorf("prompt hook: unknown builtin %q", name)
		}
		promptTemplate = tmpl
	}

	jsonBytes, err := json.Marshal(input)
	if err != nil {
		return HookOutput{}, fmt.Errorf("marshal hook input: %w", err)
	}

	prompt := strings.ReplaceAll(promptTemplate, "$ARGUMENTS", string(jsonBytes))

	tier := llm.TierFast
	if strings.EqualFold(def.Tier, "reasoning") {
		tier = llm.TierReasoning
	}

	client, _, err := b.registry.Resolve(tier)
	if err != nil {
		return HookOutput{}, fmt.Errorf("resolve %s tier: %w", tier, err)
	}

	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: prompt},
	}
	resp, err := client.Chat(ctx, msgs, nil, nil)
	if err != nil {
		return HookOutput{}, fmt.Errorf("prompt hook llm call: %w", err)
	}

	text := resp.Content
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		// JSON 응답 없음 → 허용으로 해석 (claude_cli 의 non_blocking_error 와 동등).
		return HookOutput{Decision: DecisionAllow}, nil
	}

	var result HookOutput
	if err := json.Unmarshal([]byte(text[start:end+1]), &result); err != nil {
		return HookOutput{}, fmt.Errorf("parse prompt hook output JSON: %w", err)
	}
	return result, nil
}
