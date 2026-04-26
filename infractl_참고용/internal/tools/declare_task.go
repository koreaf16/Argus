// Package tools
// File: declare_task.go
// Description: LLM이 사용자 확인 후 TaskContext 를 실제로 선언하는 도구
// Responsibility: 인자 파싱·검증 후 Manager.Declare 호출, 결과 포맷팅

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/agent/taskctx"
	"github.com/yourorg/infractl/internal/executor"
)

// DeclareTaskTool 은 사용자가 작업 제안을 확인한 후 TaskContext 를 실제로 선언할 때 호출한다.
type DeclareTaskTool struct {
	Manager *taskctx.Manager
}

func (t *DeclareTaskTool) Name() string { return "declare_task" }

func (t *DeclareTaskTool) Description() string {
	return "사용자가 작업 제안을 수락한 후 TaskContext 를 선언합니다. 이후 모든 명령은 이 컨텍스트 내에서 실행됩니다."
}

func (t *DeclareTaskTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{
				"type":        "string",
				"description": "작업 제목",
			},
			"kind": map[string]any{
				"type":        "string",
				"enum":        []string{"install", "service", "configure", "migrate", "backup", "patch", "inspect"},
				"description": "작업 유형",
			},
			"target_server": map[string]any{
				"type":        "string",
				"description": "대상 서버 호스트명 또는 IP",
			},
			"target_account": map[string]any{
				"type":        "string",
				"description": "대상 OS 계정 (선택)",
			},
			"working_dir": map[string]any{
				"type":        "string",
				"description": "작업 디렉토리 절대경로 (선택)",
			},
			"forbidden": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "금지 명령 패턴 목록 (선택)",
			},
			"preconditions": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "전제조건 목록 (선택)",
			},
			"plan": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "수행 단계 목록 (선택)",
			},
			"kind_fields": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
				"description":          "Kind 별 추가 필드 (선택)",
			},
		},
		"required": []string{"title", "kind", "target_server"},
	}
}

func (t *DeclareTaskTool) IsReadOnly() bool { return false }
func (t *DeclareTaskTool) IsEnabled() bool  { return true }

func (t *DeclareTaskTool) Execute(_ context.Context, args map[string]any, _ executor.Executor) (ToolOutcome, error) {
	if t.Manager == nil {
		return ToolOutcome{Content: "task manager not configured", Success: false}, nil
	}

	draft, err := parseDraftArgs(args)
	if err != nil {
		return ToolOutcome{Content: err.Error(), Success: false}, nil
	}

	if err := taskctx.ValidateDraft(draft); err != nil {
		return ToolOutcome{
			Content: "필수 필드 누락: " + err.Error() + "\n해당 필드를 채워서 다시 제안하세요.",
			Success: false,
		}, nil
	}

	req := draft.ToDeclareRequest()
	tc, err := t.Manager.Declare(req)
	if err != nil {
		return ToolOutcome{
			Content: fmt.Sprintf("작업 선언 실패: %s", err.Error()),
			Success: false,
		}, nil
	}

	content := formatDeclareContent(tc, draft)

	metaMap := map[string]string{
		"task_id": tc.ID,
		"title":   tc.Title,
		"kind":    string(tc.Kind),
	}
	metaBytes, err := json.Marshal(metaMap)
	if err != nil {
		return ToolOutcome{Content: fmt.Sprintf("메타데이터 직렬화 실패: %s", err), Success: false}, nil
	}

	return ToolOutcome{
		Content:      content,
		Success:      true,
		MetadataJSON: string(metaBytes),
	}, nil
}

func formatDeclareContent(tc *taskctx.TaskContext, draft taskctx.TaskContextDraft) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "작업 선언됨: %s (ID: %s)\n", tc.Title, tc.ID)
	fmt.Fprintf(&sb, "- 유형: %s\n", string(tc.Kind))
	fmt.Fprintf(&sb, "- 대상 서버: %s\n", tc.TargetServer)
	if tc.TargetAccount != "" {
		fmt.Fprintf(&sb, "- 대상 계정: %s\n", tc.TargetAccount)
	}
	if tc.WorkingDir != "" {
		fmt.Fprintf(&sb, "- 작업 디렉토리: %s\n", tc.WorkingDir)
	}

	if len(draft.PlanSteps) > 0 {
		sb.WriteString("\n수행 단계:\n")
		for i, step := range draft.PlanSteps {
			fmt.Fprintf(&sb, "%d. %s\n", i+1, step)
		}
	}

	return sb.String()
}
