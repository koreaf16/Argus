// Package subagent
// File: hooks_adapter.go
// Description: hooks.AgentRunner 인터페이스 어댑터 — *Runner를 hooks 패키지에 연결
// Responsibility: HookAgentRunner 타입 — AgentTypeIntel 서브에이전트로 hook 판단 실행

package subagent

import (
	"context"

	"github.com/yourorg/infractl/internal/hooks"
)

// HookAgentRunner는 *Runner를 hooks.AgentRunner 인터페이스에 적용하는 어댑터이다.
// hooks 패키지는 AgentRunner 인터페이스만 정의하고 구체 타입을 알지 못한다.
type HookAgentRunner struct {
	runner *Runner
	server string // 빈 문자열이면 로컬 실행 (hooks 판단은 대부분 로컬)
}

// NewHookAgentRunner는 HookAgentRunner를 생성한다.
func NewHookAgentRunner(runner *Runner) *HookAgentRunner {
	return &HookAgentRunner{runner: runner}
}

// RunForHook은 prompt를 AgentTypeHook 서브에이전트로 실행하고 결과를 반환한다.
// hooks.AgentRunner 인터페이스를 구현한다.
func (a *HookAgentRunner) RunForHook(ctx context.Context, prompt string) (string, int, error) {
	result := a.runner.Execute(ctx, SubagentConfig{
		Type:     AgentTypeHook,
		Server:   a.server,
		Question: prompt,
	})
	return result.Answer, result.ToolUses, result.Err
}

// 컴파일 타임 인터페이스 구현 검증
var _ hooks.AgentRunner = (*HookAgentRunner)(nil)
