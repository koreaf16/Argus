// Package bash — 셸 자동 완성 헬퍼.
//
// 파일 역할: 명령 입력에 대한 셸 자동 완성 후보를 반환한다.
//
//	Phase 1: 스텁 구현 (항상 nil 반환).
//	Phase 2: 실제 셸 완성 로직을 구현한다.
//
// 포함 모듈:
//   - GetShellCompletions: 명령 문자열에 대한 완성 후보 슬라이스 반환
//
// 호출/사용 방식:
//   - internal/repl 의 프롬프트 입력 컴포넌트에서 Tab 완성에 사용
//
// 연결:
//   - 원본: src/utils/bash/shellCompletion.ts
package bash

// GetShellCompletions 는 command 에 대한 셸 자동 완성 후보를 반환한다.
// Phase 1 스텁: 항상 nil 을 반환한다.
func GetShellCompletions(command string) []string {
	// TODO(Phase 2): 실제 셸 완성 로직 구현
	return nil
}
