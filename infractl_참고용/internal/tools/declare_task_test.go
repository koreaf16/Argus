// Package tools
// File: declare_task_test.go
// Description: DeclareTaskTool 실행 결과와 Manager 상태 변화 검증 테스트
// Responsibility: 선언 성공·실패 케이스, Manager nil 처리, 금지 패턴 컴파일 에러 검증

package tools

import (
	"context"
	"testing"

	"github.com/yourorg/infractl/internal/agent/taskctx"
)

func TestDeclareTaskTool_NoManager(t *testing.T) {
	tool := &DeclareTaskTool{Manager: nil}

	out, err := tool.Execute(context.Background(), map[string]any{
		"title":         "테스트 작업",
		"kind":          "inspect",
		"target_server": "srv",
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

func TestDeclareTaskTool_Success(t *testing.T) {
	mgr := taskctx.NewManager()
	tool := &DeclareTaskTool{Manager: mgr}

	out, err := tool.Execute(context.Background(), map[string]any{
		"title":          "서버 상태 점검",
		"kind":           "inspect",
		"target_server":  "db-server-01",
		"target_account": "oracle",
		"plan":           []any{"디스크 확인", "프로세스 확인"},
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Success {
		t.Fatalf("expected Success=true, got: %s", out.Content)
	}

	current := mgr.Current()
	if current == nil {
		t.Fatal("expected mgr.Current() to be non-nil after declare")
	}
	if current.Title != "서버 상태 점검" {
		t.Errorf("title mismatch: got %q", current.Title)
	}
	if current.TargetServer != "db-server-01" {
		t.Errorf("target_server mismatch: got %q", current.TargetServer)
	}
}

func TestDeclareTaskTool_Validation(t *testing.T) {
	mgr := taskctx.NewManager()
	tool := &DeclareTaskTool{Manager: mgr}

	// install kind without kind_fields — should fail validation
	out, err := tool.Execute(context.Background(), map[string]any{
		"title":         "Oracle 설치",
		"kind":          "install",
		"target_server": "db-server-01",
		// no kind_fields
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Success {
		t.Fatal("expected Success=false for install kind without kind_fields")
	}
	if mgr.Current() != nil {
		t.Error("expected no active task after failed declare")
	}
}

func TestDeclareTaskTool_BadForbidden(t *testing.T) {
	mgr := taskctx.NewManager()
	tool := &DeclareTaskTool{Manager: mgr}

	// ^[ is an invalid regex pattern
	out, err := tool.Execute(context.Background(), map[string]any{
		"title":         "점검",
		"kind":          "inspect",
		"target_server": "srv",
		"forbidden":     []any{"^["},
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Success {
		t.Fatal("expected Success=false for invalid regex in forbidden")
	}
	if mgr.Current() != nil {
		t.Error("expected no active task after compile error")
	}
}
