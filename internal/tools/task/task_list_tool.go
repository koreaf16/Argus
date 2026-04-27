package task

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/koreaf16/argus/internal/tasks"
	"github.com/koreaf16/argus/internal/tools"
)

type TaskListTool struct{}

func NewTaskListTool() *TaskListTool {
	return &TaskListTool{}
}

func (t *TaskListTool) Name() string {
	return "TaskList"
}

func (t *TaskListTool) Description(ctx tools.Context) string {
	return "List all persisted tasks with their IDs and statuses."
}

func (t *TaskListTool) InputSchema() tools.ToolInputJSONSchema {
	return tools.ToolInputJSONSchema{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (t *TaskListTool) IsReadOnly() bool {
	return true
}

func (t *TaskListTool) MaxResultSizeChars() int {
	return 10000
}

func (t *TaskListTool) Call(ctx tools.Context, input json.RawMessage) (<-chan tools.ToolEvent, error) {
	out := make(chan tools.ToolEvent, 2)
	go func() {
		defer close(out)
		list, err := tasks.ListTasks()
		if err != nil {
			out <- tools.NewErrorEvent(err)
			return
		}
		if len(list) == 0 {
			out <- tools.NewOutputEvent("No tasks found.")
			out <- tools.NewDoneEvent()
			return
		}
		var sb strings.Builder
		for _, task := range list {
			sb.WriteString(fmt.Sprintf("[%s] %s — %s\n", task.ID, task.Name, task.Status))
		}
		out <- tools.NewOutputEvent(sb.String())
		out <- tools.NewDoneEvent()
	}()
	return out, nil
}

func (t *TaskListTool) CheckPermission(ctx tools.Context, input json.RawMessage) (tools.PermissionResult, error) {
	return tools.DefaultAllowPermission(), nil
}
