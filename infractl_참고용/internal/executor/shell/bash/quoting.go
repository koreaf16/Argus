// Package bash
// File: quoting.go
// Description: POSIX 단일 인용부호 기반 셸 인자 이스케이핑
// Responsibility: Quote / QuoteCommand 공개 API

// Ported from: claude_cli/src/utils/shell/shellQuoting.ts

package bash

import "strings"

// Quote 는 s 를 POSIX 단일 인용부호로 감싸 셸 안전 문자열을 반환한다.
// 내부 단일 인용부호는 '\'' 패턴으로 이스케이핑한다.
// Ported from: claude_cli/src/utils/shell/shellQuoting.ts:quote
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// QuoteCommand 는 argv 각 토큰을 Quote 로 감싸 공백으로 조합한다.
// Ported from: claude_cli/src/utils/shell/shellQuoting.ts:quoteCommand
func QuoteCommand(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = Quote(a)
	}
	return strings.Join(parts, " ")
}
