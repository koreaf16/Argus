package task

import (
	"encoding/json"
	"fmt"

	"github.com/koreaf16/argus/internal/tasks"
	"github.com/koreaf16/argus/internal/tools"
)

type TaskUpdateTool struct{}

func NewTaskUpdateTool() *TaskUpdateTool {
	return &TaskUpdateTool{}
}

func (t *TaskUpdateTool) Name() string {
	return "TaskUpdate"
}

func (t *TaskUpdateTool) Description(ctx tools.Context) string {
	return "Update the status of an existing task by its ID."
}

func (t *TaskUpdateTool) InputSchema() tools.ToolInputJSONSchema {
	return tools.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]interface{}{
			"task_id": map[string]interface{}{"type": "string", "description": "The ID of the task to update"},
			"status":  map[string]interface{}{"type": "string", "description": "New status value (e.g. in_progress, completed, cancelled)"},
		},
		"required": []string{"task_id", "status"},
	}
}

func (t *TaskUpdateTool) IsReadOnly() bool {
	return false
}

func (t *TaskUpdateTool) MaxResultSizeChars() int {
	return 1000
}

func (t *TaskUpdateTool) Call(ctx tools.Context, input json.RawMessage) (<-chan tools.ToolEvent, error) {
	out := make(chan tools.ToolEvent, 2)

	var req struct {
		TaskID string `json:"task_id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	go func() {
		defer close(out)
		if err := tasks.UpdateTaskStatus(req.TaskID, req.Status); err != nil {
			out <- tools.NewErrorEvent(err)
			return
		}
		out <- tools.NewOutputEvent(fmt.Sprintf("Task '%s' status updated to '%s'", req.TaskID, req.Status))
		out <- tools.NewDoneEvent()
	}()
	return out, nil
}

func (t *TaskUpdateTool) CheckPermission(ctx tools.Context, input json.RawMessage) (tools.PermissionResult, error) {
	return tools.DefaultAllowPermission(), nil
}
