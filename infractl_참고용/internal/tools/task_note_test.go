// Package tools
// File: task_note_test.go
// Description: TaskNoteTool 노트 기록 및 에러 케이스 검증 테스트
// Responsibility: kind/message 필수 검증, Manager/현재 작업 없음 처리, 정상 기록 확인

package tools

import (
	"context"
	"testing"

	"github.com/yourorg/infractl/internal/agent/taskctx"
)

func TestTaskNoteTool_NoManager(t *testing.T) {
	tool := &TaskNoteTool{Manager: nil}

	out, err := tool.Execute(context.Background(), map[string]any{
		"kind":    "observation",
		"message": "서버 정상 동작 중",
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Success {
		t.Fatal("expected Success=false when Manager is nil")
	}
	if out.Content != "task manager not configured" {
		t.Errorf("unexpected content: %q", out.Content)
	}
}

func TestTaskNoteTool_NoActive(t *testing.T) {
	mgr := taskctx.NewManager()
	tool := &TaskNoteTool{Manager: mgr}

	out, err := tool.Execute(context.Background(), map[string]any{
		"kind":    "observation",
		"message": "서버 정상",
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Success {
		t.Fatal("expected Success=false when no active task")
	}
	if out.Content != "no active task" {
		t.Errorf("unexpected content: %q", out.Content)
	}
}

func TestTaskNoteTool_MissingKind(t *testing.T) {
	mgr := taskctx.NewManager()
	_, err := mgr.Declare(taskctx.DeclareRequest{
		Title:        "점검",
		Kind:         taskctx.KindInspect,
		TargetServer: "srv",
	})
	if err != nil {
		t.Fatalf("declare failed: %v", err)
	}

	tool := &TaskNoteTool{Manager: mgr}

	out, execErr := tool.Execute(context.Background(), map[string]any{
		// no kind
		"message": "점검 중",
	}, nil)

	if execErr != nil {
		t.Fatalf("unexpected error: %v", execErr)
	}
	if out.Success {
		t.Fatal("expected Success=false for missing kind")
	}
}

func TestTaskNoteTool_Success(t *testing.T) {
	mgr := taskctx.NewManager()
	tc, err := mgr.Declare(taskctx.DeclareRequest{
		Title:        "점검 작업",
		Kind:         taskctx.KindInspect,
		TargetServer: "db-server-01",
	})
	if err != nil {
		t.Fatalf("declare failed: %v", err)
	}

	tool := &TaskNoteTool{Manager: mgr}

	out, execErr := tool.Execute(context.Background(), map[string]any{
		"kind":    "checkpoint",
		"message": "1단계 완료",
	}, nil)

	if execErr != nil {
		t.Fatalf("unexpected error: %v", execErr)
	}
	if !out.Success {
		t.Fatalf("expected Success=true, got: %s", out.Content)
	}

	if len(tc.Notes) == 0 {
		t.Fatal("expected note to be appended to TaskContext")
	}
	note := tc.Notes[0]
	if note.Kind != "checkpoint" {
		t.Errorf("note kind mismatch: got %q", note.Kind)
	}
	if note.Message != "1단계 완료" {
		t.Errorf("note message mismatch: got %q", note.Message)
	}
}
