// Package tui
// File: turn_stats.go
// Description: 턴(사용자 요청 1회) 단위 통계 수집 및 요약 문자열 생성
// Responsibility: tool use 횟수·토큰·소요시간 누적 후 AgentDoneMsg 시 요약 출력

package tui

import (
	"fmt"
	"strings"
	"time"
)

// turnStats는 하나의 에이전트 턴에서 수집된 통계를 보관한다.
type turnStats struct {
	toolUses     int
	inputTokens  int
	outputTokens int
	startTime    time.Time
}

// Start는 새 턴을 시작한다.
func (s *turnStats) Start() {
	s.toolUses = 0
	s.inputTokens = 0
	s.outputTokens = 0
	s.startTime = time.Now()
}

// AddToolUse는 도구 사용 횟수를 1 증가시킨다.
func (s *turnStats) AddToolUse() {
	s.toolUses++
}

// AddTokens는 토큰 수를 누적한다.
func (s *turnStats) AddTokens(input, output int) {
	s.inputTokens += input
	s.outputTokens += output
}

// Summary는 "Done (3 tool uses · 12.5k tokens · 5.2s)" 형식의 요약 문자열을 반환한다.
// 턴이 시작되지 않은 경우(startTime.IsZero) 빈 문자열을 반환한다.
func (s *turnStats) Summary() string {
	if s.startTime.IsZero() {
		return ""
	}
	elapsed := formatElapsedShort(time.Since(s.startTime))
	totalTokens := s.inputTokens + s.outputTokens

	var parts []string
	if s.toolUses > 0 {
		parts = append(parts, fmt.Sprintf("%d tool uses", s.toolUses))
	}
	if totalTokens > 0 {
		parts = append(parts, formatTokenCount(totalTokens))
	}
	parts = append(parts, elapsed)

	inner := strings.Join(parts, " · ")
	return StyleInfoBarDim.Render("  ⎿  Done ("+inner+")")
}

// formatTokenCount는 토큰 수를 "12.5k" 형식으로 반환한다.
func formatTokenCount(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk tokens", float64(n)/1000)
	}
	return fmt.Sprintf("%d tokens", n)
}
