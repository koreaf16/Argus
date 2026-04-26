// Package tui
// File: session_state.go
// Description: 턴 간 공유되는 세션 상태 구조체
// Responsibility: 토큰 사용량, 비용, 세션 ID 등 리치 렌더링에 필요한 영속 데이터 보관

package tui

import "golang.org/x/term"

// SessionState는 에이전트 턴 간에 공유되는 상태이다.
// BubbleTea Model이 아닌 순수 데이터 구조체.
// RichModel이 생성될 때 주입받고, 종료 시 누적 데이터를 기록한다.
type SessionState struct {
	Model        string
	ServerCount  int
	TotalCostUSD float64
	InputTokens  int
	OutputTokens int
	Width        int
	SessionID    int64
}

// NewSessionState는 초기 상태를 생성한다.
func NewSessionState(model string, serverCount int) *SessionState {
	w := 80
	if fd := int(0); term.IsTerminal(fd) { // stdin fd=0
		if tw, _, err := term.GetSize(fd); err == nil && tw > 0 {
			w = tw
		}
	}
	return &SessionState{
		Model:       model,
		ServerCount: serverCount,
		Width:       w,
	}
}

// AddUsage는 LLM 호출 후 토큰/비용을 누적한다.
func (s *SessionState) AddUsage(inputTokens, outputTokens int, costUSD float64) {
	s.InputTokens += inputTokens
	s.OutputTokens += outputTokens
	s.TotalCostUSD += costUSD
}

// TotalTokens는 전체 토큰 수를 반환한다.
func (s *SessionState) TotalTokens() int {
	return s.InputTokens + s.OutputTokens
}
