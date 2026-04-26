// Package planmode
// File: approval_test.go
// Description: Approval 승인/거부/빈 큐 단위 테스트
// Responsibility: Trigger 함수의 각 분기 검증

package planmode

import (
	"context"
	"testing"
)

// stubApprovalHandler 는 테스트용 ApprovalHandler 구현체다.
type stubApprovalHandler struct {
	approvedIDs []string
	cancelled   bool
}

func (s *stubApprovalHandler) ShowApproval(_ context.Context, _ []PendingCommand) ([]string, bool) {
	return s.approvedIDs, s.cancelled
}

func TestTriggerEmptyQueue(t *testing.T) {
	res, err := Trigger(context.Background(), nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Approved) != 0 || len(res.Rejected) != 0 {
		t.Fatal("empty queue should produce empty result")
	}
}

func TestTriggerCancelled(t *testing.T) {
	cmds := []PendingCommand{
		{ID: "a", Tool: "shell_exec"},
		{ID: "b", Tool: "file_write"},
	}
	handler := &stubApprovalHandler{cancelled: true}
	res, err := Trigger(context.Background(), cmds, nil, nil, handler, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Approved) != 0 {
		t.Fatal("cancelled should approve nothing")
	}
	if len(res.Rejected) != 2 {
		t.Fatalf("expected 2 rejected, got %d", len(res.Rejected))
	}
}

func TestTriggerPartialApprove(t *testing.T) {
	cmds := []PendingCommand{
		{ID: "a", Tool: "shell_exec"},
		{ID: "b", Tool: "file_write"},
	}
	// only approve "a" — but registry=nil so execution returns error (still counts as approved)
	handler := &stubApprovalHandler{approvedIDs: []string{"a"}}
	res, err := Trigger(context.Background(), cmds, nil, nil, handler, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Approved) != 1 {
		t.Fatalf("expected 1 approved, got %d", len(res.Approved))
	}
	if res.Approved[0].ID != "a" {
		t.Fatalf("expected approved ID='a', got %q", res.Approved[0].ID)
	}
	if len(res.Rejected) != 1 || res.Rejected[0].ID != "b" {
		t.Fatal("expected rejected ID='b'")
	}
}

func TestTriggerAllRejected(t *testing.T) {
	cmds := []PendingCommand{
		{ID: "a", Tool: "shell_exec"},
	}
	handler := &stubApprovalHandler{approvedIDs: []string{}} // empty = reject all
	res, err := Trigger(context.Background(), cmds, nil, nil, handler, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Approved) != 0 {
		t.Fatal("all should be rejected")
	}
	if len(res.Rejected) != 1 {
		t.Fatal("expected 1 rejected")
	}
}
