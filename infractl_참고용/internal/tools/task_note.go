// Package tools
// File: task_note.go
// Description: LLM이 작업 진행 중 관찰·경고·체크포인트를 기록하는 도구
// Responsibility: kind/message 검증 후 Manager.Note 호출

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/agent/taskctx"
	"github.com/yourorg/infractl/internal/executor"
)

// TaskNoteTool 은 현재 활성 TaskContext 에 구조화된 노트를 추가한다.
type TaskNoteTool struct {
	Manager *taskctx.Manager
}

func (t *TaskNoteTool) Name() string { return "task_note" }

func (t *TaskNoteTool) Description() string {
	return "현재 작업 컨텍스트에 관찰·경고·체크포인트 등 구조화된 노트를 기록합니다."
}

func (t *TaskNoteTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"kind": map[string]any{
				"type":        "string",
				"description": "노트 종류 — 예: observation|warning|checkpoint|precondition_met|step_advance",
			},
			"message": map[string]any{
				"type":        "string",
				"description": "기록할 메시지",
			},
		},
		"required": []string{"kind", "message"},
	}
}

func (t *TaskNoteTool) IsReadOnly() bool { return true }
func (t *TaskNoteTool) IsEnabled() bool  { return true }

func (t *TaskNoteTool) Execute(_ context.Context, args map[string]any, _ executor.Executor) (ToolOutcome, error) {
	if t.Manager == nil {
		return ToolOutcome{Content: "task manager not configured", Success: false}, nil
	}

	if t.Manager.Current() == nil {
		return ToolOutcome{Content: "no active task", Success: false}, nil
	}

	kind, _ := args["kind"].(string)
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return ToolOutcome{Content: "kind 는 필수입니다", Success: false}, nil
	}

	message, _ := args["message"].(string)
	message = strings.TrimSpace(message)
	if message == "" {
		return ToolOutcome{Content: "message 는 필수입니다", Success: false}, nil
	}

	t.Manager.Note(kind, message)

	return ToolOutcome{
		Content: fmt.Sprintf("노트 기록됨: [%s] %s", kind, message),
		Success: true,
	}, nil
}
