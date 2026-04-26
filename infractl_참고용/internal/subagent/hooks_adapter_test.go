// Package subagent
// File: hooks_adapter_test.go
// Description: HookAgentRunner 어댑터 단위 테스트
// Responsibility: hooks.AgentRunner 인터페이스 충족 여부 컴파일 타임 검증

package subagent

import (
	"testing"

	"github.com/yourorg/infractl/internal/hooks"
)

// TestHookAgentRunner_ImplementsInterface는 HookAgentRunner가
// hooks.AgentRunner 인터페이스를 구현함을 런타임에서 재확인한다.
// 컴파일 타임 검증은 hooks_adapter.go의 var _ hooks.AgentRunner = ... 라인에서 이미 보장된다.
func TestHookAgentRunner_ImplementsInterface(t *testing.T) {
	var _ hooks.AgentRunner = NewHookAgentRunner(nil)
	// 런타임 타입 단언: 인터페이스 할당이 성공하면 테스트 통과
}
