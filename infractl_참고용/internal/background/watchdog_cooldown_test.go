// Package background
// File: watchdog_cooldown_test.go
// Description: Cooldown tests for stall watchdog notice spam prevention.
// Responsibility: Ensure repeated stall notices are suppressed during cooldown.

package background

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunStallWatchdogCooldownSuppressesSecondNotice(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "2.stdout")
	if err := os.WriteFile(path, []byte("Need confirmation [y/n]\n"), 0o600); err != nil {
		t.Fatalf("write stdout file: %v", err)
	}

	rec := newNoticeRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go RunStallWatchdog(ctx, StallWatchdogConfig{
		JobID:       2,
		StoragePath: path,
		Notifier:    rec,
		PollEvery:   20 * time.Millisecond,
		StallAfter:  60 * time.Millisecond,
		Cooldown:    250 * time.Millisecond,
		TailBytes:   512,
	})

	select {
	case <-rec.ch:
	case <-time.After(2 * time.Second):
		t.Fatal("expected first stall notice")
	}

	time.Sleep(120 * time.Millisecond)
	if got := rec.Count(); got != 1 {
		t.Fatalf("notice count during cooldown = %d, want 1", got)
	}
}
