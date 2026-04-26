// Package shell
// File: resolve.go
// Description: OS 및 $SHELL 환경변수 기반 ShellProvider 자동 선택
// Responsibility: Resolve() — 실행 환경에 맞는 프로바이더 반환

// Ported from: claude_cli/src/utils/shell/shellProvider.ts (resolveShell pattern)

package shell

import (
	"os"
	"runtime"
	"strings"
)

// Resolve 는 현재 실행 환경에 맞는 ShellProvider 를 반환한다.
// 우선순위: 등록된 $SHELL → OS 기반 기본값.
func Resolve() (ShellProvider, error) {
	if runtime.GOOS == "windows" {
		return Lookup("powershell")
	}

	if sh := os.Getenv("SHELL"); sh != "" {
		base := shellBase(sh)
		if p, err := Lookup(base); err == nil {
			return p, nil
		}
	}

	return Lookup("bash")
}

func shellBase(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}
