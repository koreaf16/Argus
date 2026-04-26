// Package background
// File: stall_watchdog_test.go
// Description: Tests for stall watchdog notice emission.
// Responsibility: Verify watchdog emits a soft stall notice without aborting work.

package background

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type noticeRecorder struct {
	mu     sync.Mutex
	events []NoticeEvent
	ch     chan NoticeEvent
}

func newNoticeRecorder() *noticeRecorder {
	return &noticeRecorder{ch: make(chan NoticeEvent, 8)}
}

func (r *noticeRecorder) Notify(ev NoticeEvent) {
	r.mu.Lock()
	r.events = append(r.events, ev)
	r.mu.Unlock()
	select {
	case r.ch <- ev:
	default:
	}
}

func (r *noticeRecorder) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

func TestRunStallWatchdogEmitsNoticeOnPromptTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "1.stdout")
	if err := os.WriteFile(path, []byte("Proceed with install? (y/n)\n"), 0o600); err != nil {
		t.Fatalf("write stdout file: %v", err)
	}

	rec := newNoticeRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go RunStallWatchdog(ctx, StallWatchdogConfig{
		JobID:       1,
		StoragePath: path,
		Notifier:    rec,
		PollEvery:   20 * time.Millisecond,
		StallAfter:  60 * time.Millisecond,
		Cooldown:    200 * time.Millisecond,
		TailBytes:   512,
	})

	select {
	case ev := <-rec.ch:
		if ev.JobID != 1 {
			t.Fatalf("job id = %d, want 1", ev.JobID)
		}
		if ev.Kind != "stall" {
			t.Fatalf("kind = %q, want stall", ev.Kind)
		}
		if len(ev.Tail) == 0 {
			t.Fatal("expected non-empty tail lines")
		}
		if !strings.Contains(strings.ToLower(strings.Join(ev.Tail, " ")), "y/n") {
			t.Fatalf("tail does not include prompt marker: %v", ev.Tail)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected stall notice, got timeout")
	}
}
