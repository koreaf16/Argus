// Package agent
// File: pending_action_test.go
// Description: PendingActionTracker의 단위 테스트
// Responsibility: Activate/Consume/Stale 생명주기 검증

package agent

import (
	"testing"
	"time"
)

func TestPendingActionTracker_ActivateConsumeLifecycle(t *testing.T) {
	tracker := newPendingActionTracker()

	if tracker.Current() != nil {
		t.Fatal("expected nil before activation")
	}

	pa := &PendingAction{
		ProposedCommand: "sudo unzip /tmp/file.zip -d /opt/",
		Reason:          "oracle 권한으로 압축 해제",
		Plan:            []string{"단계1", "단계2"},
		Source:          "tool",
	}
	tracker.Activate(pa, 5)

	cur := tracker.Current()
	if cur == nil {
		t.Fatal("expected non-nil after activation")
	}
	if cur.TurnIndex != 5 {
		t.Errorf("TurnIndex = %d, want 5", cur.TurnIndex)
	}
	if cur.ProposedCommand != pa.ProposedCommand {
		t.Errorf("ProposedCommand mismatch")
	}

	consumed := tracker.Consume()
	if consumed == nil {
		t.Fatal("Consume returned nil")
	}
	if tracker.Current() != nil {
		t.Fatal("expected nil after consume")
	}
}

func TestPendingActionTracker_ClearReason(t *testing.T) {
	tracker := newPendingActionTracker()
	tracker.Activate(&PendingAction{ProposedCommand: "ls -la", Source: "tool"}, 1)
	tracker.Clear("user_rejected")
	if tracker.Current() != nil {
		t.Fatal("expected nil after clear")
	}
}

func TestPendingActionTracker_IsStale_ByTurns(t *testing.T) {
	tracker := newPendingActionTracker()
	tracker.Activate(&PendingAction{ProposedCommand: "cmd", Source: "tool", CreatedAt: time.Now()}, 0)

	if tracker.IsStale(2) {
		t.Error("should not be stale at turn 2 (threshold=3)")
	}
	if !tracker.IsStale(3) {
		t.Error("should be stale at turn 3")
	}
}

func TestPendingActionTracker_IsStale_ByTime(t *testing.T) {
	tracker := newPendingActionTracker()
	pa := &PendingAction{
		ProposedCommand: "cmd",
		Source:          "tool",
		CreatedAt:       time.Now().Add(-6 * time.Minute),
	}
	tracker.Activate(pa, 0)
	if !tracker.IsStale(0) {
		t.Error("should be stale after 6 minutes")
	}
}

func TestPendingActionTracker_ParseFromMeta(t *testing.T) {
	tracker := newPendingActionTracker()

	meta := `{"command":"sudo unzip /tmp/a.zip","reason":"권한 문제","plan":["단계1","단계2"]}`
	pa, ok := tracker.ParseFromMeta(meta)
	if !ok {
		t.Fatal("ParseFromMeta returned false")
	}
	if pa.ProposedCommand != "sudo unzip /tmp/a.zip" {
		t.Errorf("command = %q", pa.ProposedCommand)
	}
	if len(pa.Plan) != 2 {
		t.Errorf("plan len = %d, want 2", len(pa.Plan))
	}

	_, ok = tracker.ParseFromMeta(`{}`)
	if ok {
		t.Error("empty command should return false")
	}
}
