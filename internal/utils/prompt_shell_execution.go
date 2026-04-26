// Package utils — !command 입력을 적절한 셸 도구로 라우팅.
//
// 파일 역할: 플랫폼·설정에 따라 "Bash" 또는 "PowerShell" 도구 이름을 반환한다.
//
//	REPL 이 '!' 접두사 명령을 처리할 때 어느 도구에 위임할지를 결정한다.
//
// 포함 모듈:
//   - GetShellToolForPromptExecution: 현재 환경에 맞는 셸 도구 이름 반환
//
// 호출/사용 방식:
//   - internal/repl 패키지에서 '!' 로 시작하는 사용자 입력을 도구 호출로 변환할 때 사용
//
// 연결:
//   - 원본: src/utils/promptShellExecution.ts
//   - import: internal/utils/shell (IsPowerShellToolEnabled)
package utils

import "github.com/koreaf16/argus/internal/utils/shell"

// GetShellToolForPromptExecution 은 '!' 프롬프트 입력을 실행할 셸 도구 이름을 반환한다.
// PowerShell 도구가 활성화된 경우에만 "PowerShell" 을 반환하고, 그 외엔 "Bash" 를 반환한다.
func GetShellToolForPromptExecution() string {
	if shell.IsPowerShellToolEnabled() {
		return "PowerShell"
	}
	return "Bash"
}
