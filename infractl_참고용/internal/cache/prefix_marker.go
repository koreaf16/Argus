// Package cache
// File: prefix_marker.go
// Description: 시스템 프롬프트 prefix/suffix 분할 헬퍼 — cache_control 위치 결정
// Responsibility: boundary 마커 기준으로 문자열을 두 부분으로 분리

// Ported from: claude_cli/src/utils/api.ts (splitSysPromptPrefix)

package cache

import (
	"strings"

	infracontext "github.com/yourorg/infractl/internal/context"
)

// Split 은 assembled 문자열을 DynamicBoundary 기준으로 prefix 와 suffix 로 분리한다.
// boundary 가 없으면 (assembled, "") 을 반환한다.
func Split(assembled string) (prefix, suffix string) {
	before, after, found := strings.Cut(assembled, infracontext.DynamicBoundary)
	if !found {
		return assembled, ""
	}
	return before, strings.TrimPrefix(after, "\n")
}

// HasBoundary 는 assembled 에 DynamicBoundary 가 포함되어 있는지 반환한다.
func HasBoundary(assembled string) bool {
	return strings.Contains(assembled, infracontext.DynamicBoundary)
}
