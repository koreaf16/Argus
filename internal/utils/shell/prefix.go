// Package shell — LLM 기반 명령 프리픽스 추출 (Phase 1 스텁).
//
// 파일 역할: Haiku LLM 을 사용해 명령 프리픽스를 추출하는 인터페이스를 정의한다.
//
//	Phase 1 에서는 LLM 이 없으므로 항상 nil 을 반환하는 스텁으로 구현된다.
//
// 포함 모듈:
//   - CommandPrefixResult: 프리픽스 결과 (nil 이면 사용 불가)
//   - CommandSubcommandPrefixResult: 서브커맨드 포함 복합 결과
//   - GetCommandPrefix: Phase 1 스텁 — 항상 nil 반환
//
// 호출/사용 방식:
//   - internal/tools/bash 에서 허가 사전 검사 시 호출
//   - Phase 2 에서 실제 LLM 쿼리로 교체 예정
//
// 연결:
//   - 원본: src/utils/shell/prefix.ts
package shell

import "context"

// CommandPrefixResult 는 명령 프리픽스 추출 결과다.
// CommandPrefix 가 nil 이면 프리픽스를 결정할 수 없음을 의미한다.
type CommandPrefixResult struct {
	CommandPrefix *string
}

// CommandSubcommandPrefixResult 는 서브커맨드 프리픽스를 포함한 복합 결과다.
type CommandSubcommandPrefixResult struct {
	CommandPrefixResult
	SubcommandPrefixes map[string]CommandPrefixResult
}

// GetCommandPrefix 는 명령의 프리픽스를 결정한다.
// Phase 1 스텁: LLM 을 사용하지 않으므로 항상 nil 을 반환한다.
func GetCommandPrefix(_ context.Context, _ string) (*CommandPrefixResult, error) {
	return nil, nil
}
