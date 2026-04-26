// Package planmode
// File: pending_queue.go
// Description: Plan Mode 중 차단된 명령 대기열
// Responsibility: PendingCommand 정의, Queue Add/Drain/Len/Clear

package planmode

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// PendingCommand 는 Plan Mode 중 차단된 도구 호출 하나를 나타낸다.
type PendingCommand struct {
	ID   string         // 승인/거부 식별자
	Tool string         // 도구 이름 (예: "shell_exec")
	Args map[string]any // 도구 인자 (원본 JSON 디코딩 결과)
	At   time.Time      // 차단 시각
}

// Queue 는 PendingCommand 를 thread-safe 하게 보관하는 대기열이다.
type Queue struct {
	mu    sync.Mutex
	items []PendingCommand
}

func newQueue() *Queue {
	return &Queue{}
}

// Add 는 새로운 명령을 대기열에 추가하고 부여된 ID 를 반환한다.
func (q *Queue) Add(tool string, args map[string]any) string {
	q.mu.Lock()
	defer q.mu.Unlock()
	id := uuid.New().String()
	q.items = append(q.items, PendingCommand{
		ID:   id,
		Tool: tool,
		Args: args,
		At:   time.Now(),
	})
	return id
}

// Drain 은 현재 대기열의 스냅샷을 반환하고 대기열을 비운다.
func (q *Queue) Drain() []PendingCommand {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return nil
	}
	out := make([]PendingCommand, len(q.items))
	copy(out, q.items)
	q.items = q.items[:0]
	return out
}

// Peek 은 대기열을 비우지 않고 스냅샷을 반환한다.
func (q *Queue) Peek() []PendingCommand {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]PendingCommand, len(q.items))
	copy(out, q.items)
	return out
}

// Len 은 현재 대기열 길이를 반환한다.
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// Clear 는 대기열을 비운다.
func (q *Queue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = q.items[:0]
}
