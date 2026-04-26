// Package hooks
// File: agent_runner.go
// Description: agent hook backend가 사용하는 AgentRunner 인터페이스
// Responsibility: 소비자(hooks 패키지) 측에 인터페이스 정의 — 순환 임포트 방지

package hooks

import "context"

// AgentRunner는 agent hook backend가 mini-agent를 구동하기 위한 인터페이스이다.
// 구현체는 internal/subagent.HookAgentRunner — 소비자 패키지 원칙(CLAUDE.md §6).
//
// RunForHook은 prompt를 mini-agent에 전달해 실행하고,
// 에이전트의 최종 답변 텍스트, 도구 호출 횟수, 실행 오류를 반환한다.
type AgentRunner interface {
	RunForHook(ctx context.Context, prompt string) (answer string, toolUses int, err error)
}
