// Package pending
// File: source.go
// Description: LLM 작업 도중 사용자가 보낸 추가 입력을 thread-safe하게 보관하고 drain하는 인터페이스
// Responsibility: PendingInputSource interface 정의 및 MemorySource 구현

package pending

import (
	"context"
	"sync"
	"time"
)

// PendingInput은 LLM 작업 도중 사용자가 보낸 추가 입력 한 건이다.
type PendingInput struct {
	Text string
	At   time.Time
}

// PendingInputSource는 대기 중인 사용자 입력을 원자적으로 drain하는 인터페이스다.
// TUI가 공급자, Engine이 소비자다.
type PendingInputSource interface {
	// DrainAll은 대기 중인 모든 입력을 원자적으로 반환하고 큐를 비운다.
	// 대기 중인 입력이 없으면 nil을 반환한다.
	DrainAll(ctx context.Context) []PendingInput
	// Len은 현재 대기 중인 입력 수를 반환한다.
	Len() int
}

// MemorySource는 메모리 기반 PendingInputSource 구현체다.
type MemorySource struct {
	mu      sync.Mutex
	entries []PendingInput
}

// NewMemorySource creates a new empty MemorySource.
func NewMemorySource() *MemorySource {
	return &MemorySource{}
}

// Add는 새 입력을 대기 큐에 추가한다.
func (s *MemorySource) Add(text string) {
	s.mu.Lock()
	s.entries = append(s.entries, PendingInput{Text: text, At: time.Now()})
	s.mu.Unlock()
}

// DrainAll은 대기 중인 모든 입력을 원자적으로 반환하고 큐를 비운다.
func (s *MemorySource) DrainAll(_ context.Context) []PendingInput {
	s.mu.Lock()
	all := s.entries
	s.entries = nil
	s.mu.Unlock()
	return all
}

// Len은 현재 대기 중인 입력 수를 반환한다.
func (s *MemorySource) Len() int {
	s.mu.Lock()
	n := len(s.entries)
	s.mu.Unlock()
	return n
}
