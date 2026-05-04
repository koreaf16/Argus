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
	return "구조화된 계획을 위해 플랜 모드로 진입합니다. 한 번만 사용하고, 계획 초안을 작성한 다음 완료된 계획 텍스트와 함께 exit_plan_mode를 호출하십시오."
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
			events <- tool.NewErrorEvent(fmt.Errorf("상태(state)를 사용할 수 없습니다"))
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
			"message":   "플랜 모드에 진입했습니다",
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
