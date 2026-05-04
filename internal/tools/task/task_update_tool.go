package task

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/koreaf16/argus/internal/tasks"
	"github.com/koreaf16/argus/internal/tools"
)

type TaskUpdateTool struct{}

func NewTaskUpdateTool() *TaskUpdateTool {
	return &TaskUpdateTool{}
}

func (t *TaskUpdateTool) Name() string {
	return "task_update"
}

func (t *TaskUpdateTool) Description(ctx tools.Context) string {
	return "기존 작업의 상태/제목/설명/현재진행형 문구를 갱신합니다. 작업 시작 시 status='in_progress', 완료 시 status='completed', 중단 시 'cancelled'를 사용하세요."
}

func (t *TaskUpdateTool) InputSchema() tools.ToolInputJSONSchema {
	return tools.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"task_id":     map[string]any{"type": "string", "description": "갱신할 작업의 ID"},
			"status":      map[string]any{"type": "string", "description": "새 상태 (pending|in_progress|completed|cancelled)"},
			"subject":     map[string]any{"type": "string", "description": "(선택) 새 제목"},
			"description": map[string]any{"type": "string", "description": "(선택) 새 설명"},
			"active_form": map[string]any{"type": "string", "description": "(선택) 새 진행형 문구"},
		},
		"required": []string{"task_id"},
	}
}

func (t *TaskUpdateTool) IsReadOnly() bool {
	return false
}

func (t *TaskUpdateTool) MaxResultSizeChars() int {
	return 1000
}

func (t *TaskUpdateTool) Call(ctx tools.Context, input json.RawMessage) (<-chan tools.ToolEvent, error) {
	out := make(chan tools.ToolEvent, 4)

	var req struct {
		TaskID      string  `json:"task_id"`
		Status      *string `json:"status"`
		Subject     *string `json:"subject"`
		Description *string `json:"description"`
		ActiveForm  *string `json:"active_form"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.TaskID) == "" {
		return nil, fmt.Errorf("task_id가 필요합니다")
	}

	upd := tasks.TaskUpdate{
		Name:        trimPtr(req.Subject),
		Description: trimPtr(req.Description),
		ActiveForm:  trimPtr(req.ActiveForm),
		Status:      trimPtr(req.Status),
	}

	go func() {
		defer close(out)
		if err := tasks.UpdateTaskFields(req.TaskID, upd); err != nil {
			out <- tools.NewErrorEvent(err)
			return
		}
		t, err := tasks.GetTask(req.TaskID)
		if err != nil {
			out <- tools.NewErrorEvent(err)
			return
		}
		out <- tools.NewOutputEvent(fmt.Sprintf("작업 #%d '%s' 갱신 → status=%s", t.Order, t.Name, t.Status))
		out <- tools.NewDoneEvent()
	}()
	return out, nil
}

func (t *TaskUpdateTool) CheckPermission(ctx tools.Context, input json.RawMessage) (tools.PermissionResult, error) {
	return tools.DefaultAllowPermission(), nil
}

func trimPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	return &v
}
