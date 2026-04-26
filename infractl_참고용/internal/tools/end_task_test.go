// Package tools
// File: end_task_test.go
// Description: EndTaskTool 상태 전이 및 에러 케이스 검증 테스트
// Responsibility: 종료 성공·실패, 상태 값 검증, Manager/현재 작업 없음 처리

package tools

import (
	"context"
	"testing"

	"github.com/yourorg/infractl/internal/agent/taskctx"
)

// declareInspect 는 테스트용 inspect 작업을 Manager 에 선언한다.
func declareInspect(t *testing.T, mgr *taskctx.Manager) {
	t.Helper()
	_, err := mgr.Declare(taskctx.DeclareRequest{
		Title:        "점검 작업",
		Kind:         taskctx.KindInspect,
		TargetServer: "test-srv",
	})
	if err != nil {
		t.Fatalf("failed to declare task: %v", err)
	}
}

func TestEndTaskTool_NoActive(t *testing.T) {
	mgr := taskctx.NewManager()
	tool := &EndTaskTool{Manager: mgr}

	out, err := tool.Execute(context.Background(), map[string]any{
		"status": "completed",
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Success {
		t.Fatal("expected Success=false when no active task")
	}
	if out.Content != "no active task to end" {
		t.Errorf("unexpected content: %q", out.Content)
	}
}

func TestEndTaskTool_Completed(t *testing.T) {
	mgr := taskctx.NewManager()
	declareInspect(t, mgr)
	tool := &EndTaskTool{Manager: mgr}

	out, err := tool.Execute(context.Background(), map[string]any{
		"status": "completed",
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Success {
		t.Fatalf("expected Success=true, got: %s", out.Content)
	}
	if mgr.Current() != nil {
		t.Error("expected mgr.Current() to be nil after end")
	}

	history := mgr.History()
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}
	if history[0].Status != taskctx.TaskCompleted {
		t.Errorf("expected status=completed, got %q", history[0].Status)
	}
}

func TestEndTaskTool_InvalidStatus(t *testing.T) {
	mgr := taskctx.NewManager()
	declareInspect(t, mgr)
	tool := &EndTaskTool{Manager: mgr}

	out, err := tool.Execute(context.Background(), map[string]any{
		"status": "garbage",
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Success {
		t.Fatal("expected Success=false for invalid status")
	}
	// task should still be active after failed end attempt
	if mgr.Current() == nil {
		t.Error("task should still be active after invalid status attempt")
	}
}

func TestEndTaskTool_NoManager(t *testing.T) {
	tool := &EndTaskTool{Manager: nil}

	out, err := tool.Execute(context.Background(), map[string]any{
		"status": "completed",
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Success {
		t.Fatal("expected Success=false when Manager is nil")
	}
}

func TestEndTaskTool_WithSummary(t *testing.T) {
	mgr := taskctx.NewManager()
	declareInspect(t, mgr)

	// Capture the pointer before ending so we can inspect Notes
	tc := mgr.Current()
	if tc == nil {
		t.Fatal("expected active task")
	}

	tool := &EndTaskTool{Manager: mgr}

	out, err := tool.Execute(context.Background(), map[string]any{
		"status":  "completed",
		"summary": "모든 점검 완료, 이상 없음",
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Success {
		t.Fatalf("expected Success=true, got: %s", out.Content)
	}

	// After End, current is nil but tc still holds the pointer (manager moved it to history)
	// Verify via History snapshot that the task ended as completed
	history := mgr.History()
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}
	if history[0].Status != taskctx.TaskCompleted {
		t.Errorf("expected status=completed in history, got %q", history[0].Status)
	}

	// Notes are on the *TaskContext pointer captured before End.
	// Manager appended the "end" note before clearing current.
	if len(tc.Notes) == 0 {
		t.Error("expected at least one note after end with summary")
	}
	found := false
	for _, note := range tc.Notes {
		if note.Kind == "end" && note.Message == "모든 점검 완료, 이상 없음" {
			found = true
		}
	}
	if !found {
		t.Errorf("note with kind='end' and expected message not found in Notes: %+v", tc.Notes)
	}
}
