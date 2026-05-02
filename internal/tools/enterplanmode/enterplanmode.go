// Package enterplanmode ??Enter plan mode tool implementation.
package enterplanmode

import (
	"encoding/json"
	"fmt"

	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/toolfs"
	"github.com/koreaf16/argus/internal/types"
)

type EnterPlanModeTool struct{}

func NewEnterPlanModeTool() *EnterPlanModeTool {
	return &EnterPlanModeTool{}
}

func (t *EnterPlanModeTool) Name() string {
	return "enter_plan_mode"
}

func (t *EnterPlanModeTool) Description(ctx tool.Context) string {
	return "Enter plan mode for structured planning. Use once, draft the plan, then call exit_plan_mode with the completed plan text; do not loop on TodoWrite."
}

func (t *EnterPlanModeTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *EnterPlanModeTool) IsReadOnly() bool {
	return true
}

func (t *EnterPlanModeTool) IsConcurrencySafe(input json.RawMessage) bool {
	return false
}

func (t *EnterPlanModeTool) MaxResultSizeChars() int {
	return 100000
}

func (t *EnterPlanModeTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	events := make(chan tool.ToolEvent, 2)
	go func() {
		defer close(events)

		if ctx.State == nil {
			events <- tool.NewErrorEvent(fmt.Errorf("state is unavailable"))
			return
		}

		prevMode := ctx.State.GetPermissionMode()
		if prevMode == types.PermissionModePlan {
			prevMode = types.PermissionModeDefault
		}
		ctx.State.SetPrePlanMode(prevMode)
		ctx.State.SetMode("plan")
		ctx.State.SetPermissionMode(types.PermissionModePlan)

		sessionID := ctx.State.SessionID()
		if sessionID == "" {
			sessionID = "default"
		}
		planPath, err := toolfs.EnsurePlanFile(sessionID)
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}

		payload, _ := json.Marshal(map[string]any{
			"message":   "entered plan mode",
			"plan_file": planPath,
		})
		events <- tool.NewOutputEvent(string(payload))
		events <- tool.NewDoneEvent()
	}()
	return events, nil
}

func (t *EnterPlanModeTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	return tool.DefaultAllowPermission(), nil
}
