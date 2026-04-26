// Package tools
// File: end_task.go
// Description: LLM이 현재 활성 TaskContext 를 종료할 때 호출하는 도구
// Responsibility: 상태 검증 후 Manager.End 호출, 선택적 종료 노트 기록

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/agent/taskctx"
	"github.com/yourorg/infractl/internal/executor"
)

// EndTaskTool 은 현재 활성 TaskContext 를 completed/failed/aborted 로 종료한다.
type EndTaskTool struct {
	Manager *taskctx.Manager
}

func (t *EndTaskTool) Name() string { return "end_task" }

func (t *EndTaskTool) Description() string {
	return "현재 활성 작업 컨텍스트를 종료합니다. 작업이 완료, 실패, 또는 중단되었을 때 호출하세요."
}

func (t *EndTaskTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status": map[string]any{
				"type":        "string",
				"enum":        []string{"completed", "failed", "aborted"},
				"description": "종료 상태: completed(성공)|failed(실패)|aborted(중단)",
			},
			"summary": map[string]any{
				"type":        "string",
				"description": "작업 결과 요약 노트 (선택) — Notes 에 'end' 종류로 기록됨",
			},
		},
		"required": []string{"status"},
	}
}

func (t *EndTaskTool) IsReadOnly() bool { return false }
func (t *EndTaskTool) IsEnabled() bool  { return true }

func (t *EndTaskTool) Execute(_ context.Context, args map[string]any, _ executor.Executor) (ToolOutcome, error) {
	if t.Manager == nil {
		return ToolOutcome{Content: "task manager not configured", Success: false}, nil
	}

	current := t.Manager.Current()
	if current == nil {
		return ToolOutcome{Content: "no active task to end", Success: false}, nil
	}

	statusStr, _ := args["status"].(string)
	statusStr = strings.TrimSpace(statusStr)

	switch taskctx.TaskStatus(statusStr) {
	case taskctx.TaskCompleted, taskctx.TaskFailed, taskctx.TaskAborted:
		// valid
	default:
		return ToolOutcome{
			Content: fmt.Sprintf("유효하지 않은 status: %q — completed|failed|aborted 중 하나여야 합니다", statusStr),
			Success: false,
		}, nil
	}

	summary, _ := args["summary"].(string)
	summary = strings.TrimSpace(summary)
	if summary != "" {
		t.Manager.Note("end", summary)
	}

	title := current.Title
	t.Manager.End(taskctx.TaskStatus(statusStr))

	return ToolOutcome{
		Content: fmt.Sprintf("작업 종료됨 (%s): %s", statusStr, title),
		Success: true,
	}, nil
}
