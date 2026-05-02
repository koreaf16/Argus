package todowrite

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/koreaf16/argus/internal/todostore"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/taskplaninit"
	"github.com/koreaf16/argus/internal/types"
)

type TodoWriteTool struct{}

func NewTodoWriteTool() *TodoWriteTool {
	return &TodoWriteTool{}
}

func (t *TodoWriteTool) Name() string {
	return "TodoWrite"
}

func (t *TodoWriteTool) Description(ctx tool.Context) string {
	return "Manage TODO list and workflow phases"
}

func (t *TodoWriteTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"todos": map[string]any{
				"type":        "array",
				"description": "The updated todo list",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"content":    map[string]any{"type": "string"},
						"status":     map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed"}},
						"activeForm": map[string]any{"type": "string"},
					},
					"required": []string{"content", "status", "activeForm"},
				},
			},
			"phase": map[string]any{
				"type":        "string",
				"description": "Optional: Update the current workflow phase",
				"enum":        []string{"discover", "research", "interview", "plan", "execute", "verify", "done"},
			},
		},
		"required": []string{"todos"},
	}
}

func (t *TodoWriteTool) IsReadOnly() bool {
	return false
}

func (t *TodoWriteTool) MaxResultSizeChars() int {
	return 100000
}

func (t *TodoWriteTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	events := make(chan tool.ToolEvent, 2)
	go func() {
		defer close(events)

		var req struct {
			Todos []types.TodoItem `json:"todos"`
			Phase string           `json:"phase"`
		}
		if err := json.Unmarshal(input, &req); err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}

		for i, item := range req.Todos {
			if strings.TrimSpace(item.Content) == "" {
				events <- tool.NewErrorEvent(fmt.Errorf("todos[%d].content is required", i))
				return
			}
			if strings.TrimSpace(item.ActiveForm) == "" {
				events <- tool.NewErrorEvent(fmt.Errorf("todos[%d].activeForm is required", i))
				return
			}
			switch item.Status {
			case types.TodoStatusPending, types.TodoStatusInProgress, types.TodoStatusCompleted:
			default:
				events <- tool.NewErrorEvent(fmt.Errorf("todos[%d].status is invalid", i))
				return
			}
		}

		sessionID := todostore.SessionID("")
		if ctx.State != nil {
			sessionID = todostore.SessionID(ctx.State.SessionID())
		}

		oldTodos, _ := todostore.Load(sessionID)
		if ctx.State != nil {
			stateTodos := ctx.State.Todos(sessionID)
			if len(stateTodos) > 0 {
				oldTodos = stateTodos
			}
		}

		storedTodos := todostore.NormalizeForStorage(req.Todos)
		appliedPhase := strings.TrimSpace(req.Phase)
		previousPhase := ""
		if ctx.State != nil {
			if card := ctx.State.WorkflowCard(); card != nil {
				previousPhase = strings.ToLower(strings.TrimSpace(card.Phase))
			}
			// req.Phase 가 비어 있으면 in_progress todo 로부터 phase 를 자동 추론한다.
			// 이 폴백이 없으면 LLM 이 todo status 만 갱신하고 phase 필드를 누락했을 때
			// 워크플로우가 plan 단계에 영원히 묶여 다음 도구가 모두 차단되는 데드락이 발생한다.
			if appliedPhase == "" && ctx.State.WorkflowCard() != nil {
				if inferred := taskplaninit.InferPhaseFromTodos(storedTodos); inferred != "" {
					if current := strings.ToLower(ctx.State.WorkflowCard().Phase); current != inferred {
						appliedPhase = inferred
					}
				}
			}
		}

		phaseChanged := appliedPhase != "" && !strings.EqualFold(strings.TrimSpace(appliedPhase), previousPhase)
		if reflect.DeepEqual(todostore.NormalizeForStorage(oldTodos), storedTodos) && !phaseChanged {
			events <- tool.NewErrorEvent(fmt.Errorf("TodoWrite made no changes. Do not repeat the same todo update; use the next appropriate tool (for example exit_plan_mode in plan mode) or change phase/todos explicitly."))
			return
		}

		if err := todostore.Save(sessionID, storedTodos); err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}

		if ctx.State != nil {
			ctx.State.SetTodos(sessionID, storedTodos)
			if appliedPhase != "" {
				ctx.State.SetWorkflowPhase(appliedPhase)
			}
		}

		payload, _ := json.Marshal(map[string]any{
			"oldTodos": oldTodos,
			"newTodos": storedTodos,
			"phase":    appliedPhase,
		})
		events <- tool.NewOutputEvent(string(payload))
		events <- tool.NewDoneEvent()
	}()
	return events, nil
}

func (t *TodoWriteTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	return tool.DefaultAllowPermission(), nil
}
