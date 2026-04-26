// Package context
// File: assembler.go
// Description: 정적 prefix + DynamicBoundary + 동적 suffix 결합
// Responsibility: Assemble 함수 — cache_control 분할 기준 경계 삽입

// Ported from: claude_cli/src/utils/api.ts (splitSysPromptPrefix pattern)

package context

import "strings"

// Assemble 은 정적 프롬프트(staticPrefix)와 동적 파트(dynamic)를
// DynamicBoundary 마커를 사이에 두고 결합한다.
// staticPrefix 가 비어 있으면 dynamic 만 반환한다.
func Assemble(staticPrefix, dynamic string) string {
	if strings.TrimSpace(staticPrefix) == "" {
		return dynamic
	}
	return staticPrefix + "\n" + DynamicBoundary + "\n" + dynamic
}

// SplitAtBoundary 는 Assemble 로 결합된 문자열을 prefix 와 suffix 로 분리한다.
// boundary 가 없으면 전체가 prefix 이고 suffix 는 빈 문자열이다.
func SplitAtBoundary(assembled string) (prefix, suffix string) {
	idx := strings.Index(assembled, DynamicBoundary)
	if idx < 0 {
		return assembled, ""
	}
	prefix = assembled[:idx]
	after := assembled[idx+len(DynamicBoundary):]
	suffix = strings.TrimPrefix(after, "\n")
	return prefix, suffix
}
