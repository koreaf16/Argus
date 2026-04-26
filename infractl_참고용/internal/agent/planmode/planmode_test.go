// Package planmode
// File: planmode_test.go
// Description: Plan Mode state/queue/transition 단위 테스트
// Responsibility: 상태 전환, 동시 Add/Drain 안전성, Len 일관성 검증

package planmode

import (
	"sync"
	"testing"
)

func TestStateInitial(t *testing.T) {
	s := NewState()
	if s.IsActive() {
		t.Fatal("new state should be Off")
	}
	if s.Queue().Len() != 0 {
		t.Fatal("new queue should be empty")
	}
}

func TestEnterExit(t *testing.T) {
	s := NewState()
	s.Enter()
	if !s.IsActive() {
		t.Fatal("should be Active after Enter")
	}

	// 이중 Enter 는 no-op
	s.Enter()
	if !s.IsActive() {
		t.Fatal("double Enter should remain Active")
	}

	cmds := s.Exit()
	if s.IsActive() {
		t.Fatal("should be Off after Exit")
	}
	if len(cmds) != 0 {
		t.Fatal("empty queue expected after exit with no commands")
	}
}

func TestExitDrainsQueue(t *testing.T) {
	s := NewState()
	s.Enter()
	s.Queue().Add("shell_exec", map[string]any{"command": "ls"})
	s.Queue().Add("file_write", map[string]any{"path": "/tmp/x"})

	cmds := s.Exit()
	if len(cmds) != 2 {
		t.Fatalf("expected 2 pending commands, got %d", len(cmds))
	}
	if s.Queue().Len() != 0 {
		t.Fatal("queue should be empty after Exit")
	}
}

func TestToggle(t *testing.T) {
	s := NewState()
	if active := s.Toggle(); !active {
		t.Fatal("Toggle from Off should return true")
	}
	if !s.IsActive() {
		t.Fatal("should be Active")
	}
	if active := s.Toggle(); active {
		t.Fatal("Toggle from Active should return false")
	}
	if s.IsActive() {
		t.Fatal("should be Off")
	}
}

func TestQueueConcurrent(t *testing.T) {
	q := newQueue()
	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			q.Add("tool", map[string]any{"i": i})
		}()
	}
	wg.Wait()

	if q.Len() != n {
		t.Fatalf("expected %d items, got %d", n, q.Len())
	}

	drained := q.Drain()
	if len(drained) != n {
		t.Fatalf("Drain expected %d, got %d", n, len(drained))
	}
	if q.Len() != 0 {
		t.Fatal("queue should be empty after Drain")
	}
}

func TestQueuePeekDoesNotDrain(t *testing.T) {
	q := newQueue()
	q.Add("tool", nil)
	_ = q.Peek()
	if q.Len() != 1 {
		t.Fatal("Peek should not remove items")
	}
}
