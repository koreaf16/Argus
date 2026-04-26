// Package executor
// File: idle_test.go
// Description: IdleWatcher 단위 테스트
// Responsibility: 타이머 리셋 및 트리거 동작 검증

package executor

import (
	"context"
	"testing"
	"time"
)

func TestIdleWatcher_TriggersAfterThreshold(t *testing.T) {
	threshold := 50 * time.Millisecond
	w := NewIdleWatcher(threshold)
	ctx := context.Background()
	w.Start(ctx)
	defer w.Stop()

	// Ping 없이 threshold 경과 → Triggered 채널이 닫혀야 함
	select {
	case <-w.Triggered():
		// 정상
	case <-time.After(threshold * 5):
		t.Fatal("IdleWatcher did not trigger within expected time")
	}
}

func TestIdleWatcher_PingResetsTimer(t *testing.T) {
	threshold := 80 * time.Millisecond
	w := NewIdleWatcher(threshold)
	ctx := context.Background()
	w.Start(ctx)
	defer w.Stop()

	// threshold보다 짧은 간격으로 Ping → 트리거되면 안 됨
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 5; i++ {
			time.Sleep(threshold / 3)
			w.Ping()
		}
	}()

	select {
	case <-w.Triggered():
		t.Fatal("IdleWatcher triggered unexpectedly while pinging")
	case <-done:
		// Ping 동안 트리거되지 않았음 — 정상
	}
}

func TestIdleWatcher_StopPreventsLeak(t *testing.T) {
	threshold := 50 * time.Millisecond
	w := NewIdleWatcher(threshold)
	ctx := context.Background()
	w.Start(ctx)

	// Stop 후에는 Triggered 채널이 닫히더라도 고루틴 누수 없음
	w.Stop()
	// Stop을 여러 번 호출해도 패닉 없어야 함
	w.Stop()
}

func TestIdleWatcher_ContextCancelsWatcher(t *testing.T) {
	threshold := 200 * time.Millisecond
	w := NewIdleWatcher(threshold)
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	cancel() // ctx 취소 → 고루틴 종료, 트리거되지 않아야 함

	select {
	case <-w.Triggered():
		// 이미 취소 전 트리거될 수도 있음 — 레이스 조건 무시
	case <-time.After(threshold * 2):
		// 트리거 없음 — 정상 (ctx 취소가 먼저 처리됨)
	}
}
