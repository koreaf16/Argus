// Package query
// File: concurrency.go
// Description: 최대 동시 도구 실행 수 결정
// Responsibility: 환경변수 기반 max concurrency 조회
//
// Ported from: claude_cli/src/services/tools/toolOrchestration.ts:8-12
// 핵심 패턴: getMaxToolUseConcurrency — env fallback 10

package query

import (
	"os"
	"strconv"
)

const defaultMaxConcurrency = 10

// MaxToolConcurrency 는 도구 동시 실행 최대 수를 반환한다.
// 환경변수 INFRACTL_MAX_TOOL_CONCURRENCY 가 유효한 양의 정수이면 그 값을 사용하고,
// 그렇지 않으면 기본값 10 을 사용한다.
// claude_cli: getMaxToolUseConcurrency (toolOrchestration.ts:8-12)
func MaxToolConcurrency() int {
	raw := os.Getenv("INFRACTL_MAX_TOOL_CONCURRENCY")
	if raw == "" {
		return defaultMaxConcurrency
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultMaxConcurrency
	}
	return n
}
