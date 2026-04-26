// Package planmode
// File: mode.go
// Description: Plan Mode 상태 정의 및 세션 단위 State 관리
// Responsibility: Mode enum, State 구조체, thread-safe getter/setter

// Ported from: claude_cli/src/tools/EnterPlanModeTool/EnterPlanModeTool.ts (mode transition 패턴)
// 핵심 패턴: toolPermissionContext.mode = 'plan' 상태를 Go struct으로 매핑,
//            RWMutex 로 동시 접근 보호.

package planmode

import "sync"

// Mode 는 Plan Mode 의 활성화 상태다.
type Mode uint8

const (
	// Off 는 Plan Mode 가 비활성화된 상태다.
	Off Mode = iota
	// Active 는 Plan Mode 가 활성화된 상태 — mutation 도구 호출이 차단된다.
	Active
)

// State 는 세션 단위 Plan Mode 상태를 thread-safe 하게 보관한다.
type State struct {
	mu    sync.RWMutex
	mode  Mode
	queue *Queue
}

// NewState 는 Off 상태의 State 를 반환한다.
func NewState() *State {
	return &State{
		mode:  Off,
		queue: newQueue(),
	}
}

// Mode 는 현재 Plan Mode 를 반환한다.
func (s *State) Mode() Mode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mode
}

// IsActive 는 Plan Mode 가 활성화 중인지 반환한다.
func (s *State) IsActive() bool {
	return s.Mode() == Active
}

// setMode 는 내부 전환 전용 setter 다.
func (s *State) setMode(m Mode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode = m
}

// Queue 는 Plan Mode 가 차단한 명령 대기열을 반환한다.
func (s *State) Queue() *Queue {
	return s.queue
}
