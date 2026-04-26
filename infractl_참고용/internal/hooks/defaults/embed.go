// Package defaults
// File: embed.go
// Description: hooks.yaml.default 를 //go:embed 로 내장하고 반환한다.
// Responsibility: 기본 정책 YAML 접근 API (infrainit 부트스트랩이 참조).

package defaults

import (
	_ "embed"
	"regexp"
	"runtime"
)

//go:embed hooks.yaml.default
var defaultYAML []byte

var (
	shExtRegex      = regexp.MustCompile(`\.sh(["'])`)
	commandPathRegex = regexp.MustCompile(`(command:\s*)"([^"]+system_risk\.ps1)"`)
)

// DefaultHooksYAML 은 기본 hooks.yaml 내용(바이트)을 반환한다.
// Windows 환경인 경우 내장 .sh 스크립트 참조를 .ps1 호출로 자동 전환한다.
func DefaultHooksYAML() []byte {
	if runtime.GOOS == "windows" {
		content := string(defaultYAML)
		// 1) .sh -> .ps1 치환 (인용구 유지)
		content = shExtRegex.ReplaceAllString(content, ".ps1$1")

		// 2) PowerShell 호출 연산자(&)를 사용하여 스크립트 호출 형식으로 변환
		// backend_command 가 이제 powershell -Command 를 직접 호출하므로, 커맨드 내용은 PS 문법에 맞춤.
		// 예: command: "~/.../system_risk.ps1" -> command: "& '~/.../system_risk.ps1'"
		content = commandPathRegex.ReplaceAllString(content, `$1"& '$2'"`)

		return []byte(content)
	}

	out := make([]byte, len(defaultYAML))
	copy(out, defaultYAML)
	return out
}
