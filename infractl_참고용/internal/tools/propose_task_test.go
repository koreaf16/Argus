// Package tools
// File: propose_task_test.go
// Description: ProposeTaskTool 입출력 형식 및 검증 로직 테스트
// Responsibility: propose_task 실행 결과와 MetadataJSON 직렬화 정확성 검증

package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/agent/taskctx"
)

// installKindFields 는 install kind 에 필요한 모든 필수 필드를 포함한다.
func installKindFields() map[string]any {
	return map[string]any{
		"software_name":      "Oracle Database",
		"software_version":   "19.3.0",
		"install_method":     "official_installer",
		"install_path":       "/u01/app/oracle/product/19.3.0/dbhome_1",
		"install_options":    "silent install with response file",
		"owner_account":      "oracle",
		"post_install_tasks": "DB 생성, 리스너 설정",
	}
}

func TestProposeTaskTool_ValidInstall(t *testing.T) {
	tool := &ProposeTaskTool{}

	out, err := tool.Execute(context.Background(), map[string]any{
		"title":         "Oracle 19c 설치",
		"kind":          "install",
		"target_server": "db-server-01",
		"kind_fields":   installKindFields(),
		"risk_analysis": "DB 서비스 중단 없이 신규 설치이므로 위험 낮음",
		"rollback_plan": []any{"설치 디렉토리 삭제", "환경변수 원복"},
		"reasoning":     "운영 DB 버전 업그레이드를 위한 신규 홈 설치",
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Success {
		t.Fatalf("expected Success=true, got Content: %s", out.Content)
	}
	if !strings.Contains(out.MetadataJSON, "install") {
		t.Errorf("MetadataJSON should contain 'install', got: %s", out.MetadataJSON)
	}
	if !strings.Contains(out.Content, "확인하시면") {
		t.Error("content should contain confirmation prompt")
	}
}

func TestProposeTaskTool_MissingKindFields(t *testing.T) {
	tool := &ProposeTaskTool{}

	out, err := tool.Execute(context.Background(), map[string]any{
		"title":         "Oracle 설치",
		"kind":          "install",
		"target_server": "db-server-01",
		// kind_fields 없음
		"risk_analysis": "위험 낮음",
		"rollback_plan": []any{"원복"},
		"reasoning":     "필요해서",
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Success {
		t.Fatal("expected Success=false for missing kind_fields")
	}
	// 누락 필드 이름이 응답에 포함되어야 한다
	if !strings.Contains(out.Content, "software_name") &&
		!strings.Contains(out.Content, "누락") &&
		!strings.Contains(out.Content, "missing") {
		t.Errorf("content should mention missing field, got: %s", out.Content)
	}
}

func TestProposeTaskTool_MissingRequired(t *testing.T) {
	tool := &ProposeTaskTool{}

	// title 없음
	out, err := tool.Execute(context.Background(), map[string]any{
		"kind":          "inspect",
		"target_server": "srv",
		"risk_analysis": "없음",
		"rollback_plan": []any{"없음"},
		"reasoning":     "점검",
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Success {
		t.Fatal("expected Success=false for missing title")
	}
}

func TestProposeTaskTool_InspectKind(t *testing.T) {
	tool := &ProposeTaskTool{}

	out, err := tool.Execute(context.Background(), map[string]any{
		"title":         "서버 상태 점검",
		"kind":          "inspect",
		"target_server": "db-server-01",
		// inspect 는 kind_fields 필수 없음
		"risk_analysis": "읽기 전용 작업으로 위험 없음",
		"rollback_plan": []any{"없음"},
		"reasoning":     "정기 점검",
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Success {
		t.Fatalf("expected Success=true for inspect kind, got: %s", out.Content)
	}
}

func TestProposeTaskTool_MetadataRoundtrip(t *testing.T) {
	tool := &ProposeTaskTool{}

	out, err := tool.Execute(context.Background(), map[string]any{
		"title":         "Oracle 19c 설치",
		"kind":          "install",
		"target_server": "db-server-01",
		"target_account": "oracle",
		"kind_fields":   installKindFields(),
		"risk_analysis": "위험 낮음",
		"rollback_plan": []any{"설치 디렉토리 삭제"},
		"reasoning":     "업그레이드",
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Success {
		t.Fatalf("expected Success=true, got: %s", out.Content)
	}

	var meta ProposeTaskMeta
	if err := json.Unmarshal([]byte(out.MetadataJSON), &meta); err != nil {
		t.Fatalf("MetadataJSON is not valid JSON: %v\nraw: %s", err, out.MetadataJSON)
	}

	if meta.Draft.Title != "Oracle 19c 설치" {
		t.Errorf("draft title mismatch: got %q", meta.Draft.Title)
	}
	if meta.Draft.Kind != taskctx.KindInstall {
		t.Errorf("draft kind mismatch: got %q", meta.Draft.Kind)
	}
	if meta.Draft.TargetServer != "db-server-01" {
		t.Errorf("draft target_server mismatch: got %q", meta.Draft.TargetServer)
	}
	if meta.RiskAnalysis != "위험 낮음" {
		t.Errorf("risk_analysis mismatch: got %q", meta.RiskAnalysis)
	}
	if len(meta.RollbackPlan) != 1 || meta.RollbackPlan[0] != "설치 디렉토리 삭제" {
		t.Errorf("rollback_plan mismatch: got %v", meta.RollbackPlan)
	}
}
