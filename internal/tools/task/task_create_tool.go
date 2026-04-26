package task

import (
	"encoding/json"
	"fmt"

	"github.com/koreaf16/argus/internal/tasks"
	"github.com/koreaf16/argus/internal/tools"
)

type TaskCreateTool struct{}

func NewTaskCreateTool() *TaskCreateTool {
	return &TaskCreateTool{}
}

func (t *TaskCreateTool) Name() string {
	return "TaskCreate"
}

func (t *TaskCreateTool) Description(ctx tools.Context) string {
	return "Create a new task and persist it to the system."
}

func (t *TaskCreateTool) InputSchema() tools.ToolInputJSONSchema {
	return tools.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]interface{}{
			"task_name": map[string]interface{}{"type": "string", "description": "The name of the task to create"},
		},
		"required": []string{"task_name"},
	}
}

func (t *TaskCreateTool) IsReadOnly() bool {
	return false
}

func (t *TaskCreateTool) Call(ctx tools.Context, input json.RawMessage) (<-chan tools.ToolEvent, error) {
	out := make(chan tools.ToolEvent, 2)

	var req struct {
		TaskName string `json:"task_name"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	go func() {
		defer close(out)
		if err := tasks.SaveTask(req.TaskName); err != nil {
			out <- tools.NewErrorEvent(err)
			return
		}
		out <- tools.NewOutputEvent(fmt.Sprintf("Task '%s' created and saved successfully", req.TaskName))
		out <- tools.NewDoneEvent()
	}()

	return out, nil
}

func (t *TaskCreateTool) CheckPermission(ctx tools.Context, input json.RawMessage) (tools.PermissionResult, error) {
	return tools.DefaultAllowPermission(), nil
}

func (t *TaskCreateTool) MaxResultSizeChars() int {
	return 1000
}
