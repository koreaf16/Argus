package todoread

import (
	"encoding/json"

	"github.com/koreaf16/argus/internal/todostore"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
)

type TodoReadTool struct{}

func NewTodoReadTool() *TodoReadTool {
	return &TodoReadTool{}
}

func (t *TodoReadTool) Name() string {
	return "TodoRead"
}

func (t *TodoReadTool) Description(ctx tool.Context) string {
	return "Read the current TODO list"
}

func (t *TodoReadTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *TodoReadTool) IsReadOnly() bool {
	return true
}

func (t *TodoReadTool) MaxResultSizeChars() int {
	return 100000
}

func (t *TodoReadTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	events := make(chan tool.ToolEvent, 2)
	go func() {
		defer close(events)

		sessionID := ""
		if ctx.State != nil {
			sessionID = ctx.State.SessionID()
		}

		var todos []types.TodoItem
		if ctx.State != nil {
			todos = ctx.State.Todos(sessionID)
		}
		if len(todos) == 0 {
			var err error
			todos, err = todostore.Load(sessionID)
			if err != nil {
				events <- tool.NewErrorEvent(err)
				return
			}
		}

		if todos == nil {
			todos = []types.TodoItem{}
		}

		payload, _ := json.Marshal(map[string]any{
			"todos": todos,
		})
		events <- tool.NewOutputEvent(string(payload))
		events <- tool.NewDoneEvent()
	}()
	return events, nil
}

func (t *TodoReadTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	return tool.DefaultAllowPermission(), nil
}
