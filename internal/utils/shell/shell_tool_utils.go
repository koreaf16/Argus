// Package shell — 셸 도구 이름 목록과 PowerShell 도구 활성화 여부 판별.
//
// 파일 역할: 셸 도구 이름 슬라이스와 IsPowerShellToolEnabled 함수를 제공한다.
//
//	PowerShell 도구는 Windows 에서만 사용 가능하며, 환경변수로 제어된다.
//
// 포함 모듈:
//   - ShellToolNames: ["Bash", "PowerShell"] 도구 이름 슬라이스
//   - IsPowerShellToolEnabled: Windows + 환경변수 조건을 검사해 활성화 여부 반환
//
// 호출/사용 방식:
//   - internal/tool/tools.go 에서 도구 목록 가시성 결정 시 호출
//   - internal/tools/bash/bash_tool.go 에서 '!' 라우팅 시 호출
//
// 연결:
//   - 원본: src/utils/shell/shellToolUtils.ts
package shell

import (
	"os"
	"runtime"
	"strings"
)

// ShellToolNames 는 등록된 셸 도구 이름 목록이다.
var ShellToolNames = []string{"Bash", "PowerShell"}

// IsPowerShellToolEnabled 는 PowerShell 도구를 활성화해야 하는지 반환한다.
// Windows 에서만 활성화되며, CLAUDE_CODE_USE_POWERSHELL_TOOL 환경변수가
// truthy("1","true","yes","on")일 때만 true 를 반환한다.
func IsPowerShellToolEnabled() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	return isTruthy(os.Getenv("CLAUDE_CODE_USE_POWERSHELL_TOOL"))
}

// isTruthy 는 값이 "1", "true", "yes", "on" 중 하나(대소문자 무시)이면 true 를 반환한다.
func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
