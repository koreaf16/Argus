// Package tui
// File: pending_bridge.go
// Description: inputQueue를 pending.PendingInputSource로 노출하는 어댑터
// Responsibility: TUI inputQueue ↔ Engine PendingInputSource 변환 단일 책임

package tui

import (
	"context"

	"github.com/yourorg/infractl/internal/agent/pending"
)

// pendingBridge는 *inputQueue를 pending.PendingInputSource로 적응시킨다.
// DrainAll은 큐를 원자적으로 비우므로 mid-turn 주입 후 post-turn dequeue는 noop이 된다.
type pendingBridge struct {
	queue *inputQueue
}

func newPendingBridge(q *inputQueue) *pendingBridge {
	return &pendingBridge{queue: q}
}

// DrainAll은 큐의 모든 항목을 원자적으로 꺼내 PendingInput 슬라이스로 변환한다.
func (b *pendingBridge) DrainAll(_ context.Context) []pending.PendingInput {
	var entries []queueEntry
	for {
		e, ok := b.queue.Dequeue()
		if !ok {
			break
		}
		entries = append(entries, e)
	}
	
	if len(entries) == 0 {
		return nil
	}
	result := make([]pending.PendingInput, len(entries))
	for i, e := range entries {
		result[i] = pending.PendingInput{Text: e.expandedInput, At: e.queuedAt}
	}
	return result
}

// Len은 현재 큐에 대기 중인 항목 수를 반환한다.
func (b *pendingBridge) Len() int {
	return b.queue.Len()
}
