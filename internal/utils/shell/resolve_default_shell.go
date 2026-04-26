// Package shell — 기본 셸 타입 결정 함수.
//
// 파일 역할: ARGUS_DEFAULT_SHELL 환경변수를 읽어 기본 셸 타입을 반환한다.
//
//	미설정이거나 인식 불가한 값이면 bash 를 반환한다.
//
// 포함 모듈:
//   - ResolveDefaultShell: 기본 셸 타입(ShellType) 반환
//
// 호출/사용 방식:
//   - internal/tools/bash 및 REPL 진입 시 기본 셸 결정에 사용
//
// 연결:
//   - 원본: src/utils/shell/resolveDefaultShell.ts
package shell

import (
	"os"
	"strings"
)

// ResolveDefaultShell 은 ARGUS_DEFAULT_SHELL 환경변수를 읽어 기본 셸 타입을 반환한다.
// 유효한 값: "bash", "powershell". 미설정이거나 인식 불가한 값이면 ShellTypeBash 를 반환한다.
func ResolveDefaultShell() ShellType {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ARGUS_DEFAULT_SHELL")))
	switch v {
	case "powershell":
		return ShellTypePS
	default:
		return ShellTypeBash
	}
}
