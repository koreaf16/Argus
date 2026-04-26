// Package context
// File: boundary.go
// Description: 시스템 프롬프트 prefix/dynamic 경계 마커 상수
// Responsibility: DynamicBoundary 값 단일 정의 — cache_control 분할 기준점

package context

// DynamicBoundary 는 정적 prefix 와 동적 suffix 사이의 경계 마커이다.
// Ported from: claude_cli/src/utils/api.ts (DYNAMIC_BOUNDARY)
const DynamicBoundary = "__INFRACTL_DYNAMIC_BOUNDARY__"
