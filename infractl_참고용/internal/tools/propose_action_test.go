// Package tools
// File: propose_action_test.go
// Description: ProposeActionTool 입출력 형식 검증 테스트
// Responsibility: 도구 실행 결과가 표준 제안 포맷을 갖는지 검증

package tools

import (
	"context"
	"strings"
	"testing"
)

func TestProposeActionTool_BasicExecution(t *testing.T) {
	tool := &ProposeActionTool{}

	got, err := tool.Execute(context.Background(), map[string]interface{}{
		"command": "sudo unzip /home/oracle/LINUX.X64_193000_db_home.zip -d /app/oracle/product/19c/dbhome_1",
		"reason":  "oracle 사용자 권한으로 압축 해제",
		"plan":    []interface{}{"DB Home 압축 해제", "OPatch 교체", "Installer 실행"},
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Content == "" {
		t.Fatal("expected non-empty content")
	}
	if !strings.Contains(got.Content, "계속 진행하시겠습니까") {
		t.Error("content should contain confirmation prompt")
	}
	if !strings.Contains(got.Content, "unzip") {
		t.Error("content should contain the command")
	}
	if !strings.Contains(got.Content, "OPatch 교체") {
		t.Error("content should contain plan steps")
	}
	if got.MetadataJSON == "" {
		t.Fatal("expected non-empty MetadataJSON")
	}
}

func TestProposeActionTool_MissingCommand(t *testing.T) {
	tool := &ProposeActionTool{}

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"reason": "이유만 있음",
	}, nil)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out.Success {
		t.Fatal("expected Success=false for missing command")
	}
}

func TestProposeActionTool_NoPlan(t *testing.T) {
	tool := &ProposeActionTool{}

	got, err := tool.Execute(context.Background(), map[string]interface{}{
		"command": "systemctl restart nginx",
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Content == "" {
		t.Fatal("expected non-empty content")
	}
}

func TestProposeActionTool_Metadata(t *testing.T) {
	tool := &ProposeActionTool{}

	got, err := tool.Execute(context.Background(), map[string]interface{}{
		"command": "ls -la",
		"reason":  "디렉토리 확인",
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got.MetadataJSON, "ls -la") {
		t.Errorf("MetadataJSON should contain command, got: %s", got.MetadataJSON)
	}
}
