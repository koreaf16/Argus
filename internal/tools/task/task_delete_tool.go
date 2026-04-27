package task

import (
	"encoding/json"
	"fmt"

	"github.com/koreaf16/argus/internal/tasks"
	"github.com/koreaf16/argus/internal/tools"
)

type TaskDeleteTool struct{}

func NewTaskDeleteTool() *TaskDeleteTool {
	return &TaskDeleteTool{}
}

func (t *TaskDeleteTool) Name() string {
	return "TaskDelete"
}

func (t *TaskDeleteTool) Description(ctx tools.Context) string {
	return "Delete a persisted task by its ID."
}

func (t *TaskDeleteTool) InputSchema() tools.ToolInputJSONSchema {
	return tools.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]interface{}{
			"task_id": map[string]interface{}{"type": "string", "description": "The ID of the task to delete"},
		},
		"required": []string{"task_id"},
	}
}

func (t *TaskDeleteTool) IsReadOnly() bool {
	return false
}

func (t *TaskDeleteTool) MaxResultSizeChars() int {
	return 1000
}

func (t *TaskDeleteTool) Call(ctx tools.Context, input json.RawMessage) (<-chan tools.ToolEvent, error) {
	out := make(chan tools.ToolEvent, 2)

	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	go func() {
		defer close(out)
		if err := tasks.DeleteTask(req.TaskID); err != nil {
			out <- tools.NewErrorEvent(err)
			return
		}
		out <- tools.NewOutputEvent(fmt.Sprintf("Task '%s' deleted successfully", req.TaskID))
		out <- tools.NewDoneEvent()
	}()
	return out, nil
}

func (t *TaskDeleteTool) CheckPermission(ctx tools.Context, input json.RawMessage) (tools.PermissionResult, error) {
	return tools.DefaultAllowPermission(), nil
}
