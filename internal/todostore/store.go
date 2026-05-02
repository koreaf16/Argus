package todostore

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/koreaf16/argus/internal/tools/toolfs"
	"github.com/koreaf16/argus/internal/types"
)

type Step struct {
	Tool   string
	Prompt string
}

func SessionID(raw string) string {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "default"
	}
	return id
}

func Load(sessionID string) ([]types.TodoItem, error) {
	data, err := toolfs.ReadIfExists(toolfs.TodoPath(SessionID(sessionID)))
	if err != nil || len(data) == 0 {
		return nil, err
	}
	var todos []types.TodoItem
	if err := json.Unmarshal(data, &todos); err != nil {
		return nil, err
	}
	return todos, nil
}

func Save(sessionID string, todos []types.TodoItem) error {
	stored := NormalizeForStorage(todos)
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	return toolfs.AtomicWrite(toolfs.TodoPath(SessionID(sessionID)), data, 0o600)
}

func NormalizeForStorage(todos []types.TodoItem) []types.TodoItem {
	if len(todos) == 0 {
		return nil
	}
	allDone := true
	for _, item := range todos {
		if item.Status != types.TodoStatusCompleted {
			allDone = false
			break
		}
	}
	if allDone {
		return nil
	}
	cp := make([]types.TodoItem, len(todos))
	copy(cp, todos)
	return cp
}

func SyncForSteps(existing []types.TodoItem, steps []Step) []types.TodoItem {
	if len(steps) == 0 {
		return nil
	}
	out := make([]types.TodoItem, 0, len(steps))
	if len(existing) == len(steps) {
		for i := range steps {
			next := todoForStep(i, steps[i])
			if existing[i].Content == next.Content {
				next.ActiveForm = existing[i].ActiveForm
			}
			out = append(out, next)
		}
		return out
	}
	for i, step := range steps {
		out = append(out, todoForStep(i, step))
	}
	return out
}

func todoForStep(index int, step Step) types.TodoItem {
	return types.TodoItem{
		Content:    fmt.Sprintf("%s: %s", step.Tool, step.Prompt),
		ActiveForm: fmt.Sprintf("running step %d", index+1),
		Status:     types.TodoStatusPending,
	}
}
