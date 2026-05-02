package todoread

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/koreaf16/argus/internal/state"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
)

func TestTodoReadPrefersStateTodos(t *testing.T) {
	st := state.NewAppState()
	st.SetSessionID("todo-read-state")
	st.SetTodos("todo-read-state", []types.TodoItem{
		{Content: "from state", Status: types.TodoStatusInProgress, ActiveForm: "reading state"},
	})

	events, err := NewTodoReadTool().Call(tool.Context{Context: context.Background(), State: st}, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var output string
	for evt := range events {
		if evt.Kind == tool.ToolEventOutput {
			output = evt.Output
		}
		if evt.Kind == tool.ToolEventError {
			t.Fatalf("tool error: %v", evt.Err)
		}
	}
	if !strings.Contains(output, "from state") {
		t.Fatalf("expected state todo in output, got %s", output)
	}
}
