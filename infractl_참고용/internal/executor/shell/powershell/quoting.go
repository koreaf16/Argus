// Package powershell
// File: quoting.go
// Description: PowerShell 문자열 이스케이핑
// Responsibility: Quote — 단일 인용부호로 변수 보간 및 특수문자 차단

// Ported from: claude_cli/src/utils/shell/powershellProvider.ts (quoting section)

package powershell

import "strings"

// Quote 는 s 를 PowerShell 단일 인용부호로 감싸 변수 보간을 차단한다.
// PowerShell 에서 단일 인용부호 안의 ' 는 '' 로 이스케이핑한다.
// Ported from: claude_cli/src/utils/shell/powershellProvider.ts
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
