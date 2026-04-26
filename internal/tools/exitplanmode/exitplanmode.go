// Package exitplanmode ??Exit plan mode tool implementation.
package exitplanmode

import (
	"encoding/json"
	"fmt"
	"strings"

	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/toolfs"
	"github.com/koreaf16/argus/internal/types"
)

type ExitPlanModeTool struct{}

func NewExitPlanModeTool() *ExitPlanModeTool {
	return &ExitPlanModeTool{}
}

func (t *ExitPlanModeTool) Name() string {
	return "exit_plan_mode"
}

func (t *ExitPlanModeTool) Description(ctx tool.Context) string {
	return "Exit plan mode"
}

func (t *ExitPlanModeTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"allowedPrompts": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"tool":   map[string]any{"type": "string"},
						"prompt": map[string]any{"type": "string"},
					},
					"required": []string{"tool", "prompt"},
				},
			},
		},
	}
}

func (t *ExitPlanModeTool) IsReadOnly() bool {
	return true
}

func (t *ExitPlanModeTool) MaxResultSizeChars() int {
	return 100000
}

func (t *ExitPlanModeTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	events := make(chan tool.ToolEvent, 2)
	go func() {
		defer close(events)
		if ctx.State == nil {
			events <- tool.NewErrorEvent(fmt.Errorf("state is unavailable"))
			return
		}

		var req struct {
			AllowedPrompts []struct {
				Tool   string `json:"tool"`
				Prompt string `json:"prompt"`
			} `json:"allowedPrompts"`
		}
		_ = json.Unmarshal(input, &req)
		allowedPrompts := normalizeAllowedPrompts(req.AllowedPrompts)

		sessionID := ctx.State.SessionID()
		if sessionID == "" {
			sessionID = "default"
		}
		planText, planPath, err := toolfs.LoadPlan(sessionID)
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		if len(allowedPrompts) > 0 {
			planText = renderPlanSteps(planText, allowedPrompts)
			planPath, err = toolfs.WritePlan(sessionID, planText)
			if err != nil {
				events <- tool.NewErrorEvent(err)
				return
			}
		}

		restore := ctx.State.PrePlanMode()
		if restore == "" || restore == types.PermissionModePlan {
			restore = types.PermissionModeDefault
		}
		ctx.State.SetPermissionMode(restore)
		ctx.State.SetMode("normal")
		ctx.State.ClearPrePlanMode()

		payload, _ := json.Marshal(map[string]any{
			"message":               "exited plan mode",
			"restored_mode":         string(restore),
			"plan":                  planText,
			"plan_file":             planPath,
			"allowed_prompts":       allowedPrompts,
			"allowed_prompts_count": len(allowedPrompts),
		})
		events <- tool.NewOutputEvent(string(payload))
		events <- tool.NewDoneEvent()
	}()
	return events, nil
}

func (t *ExitPlanModeTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	return tool.DefaultAllowPermission(), nil
}

type allowedPrompt struct {
	Tool   string `json:"tool"`
	Prompt string `json:"prompt"`
}

func normalizeAllowedPrompts(items []struct {
	Tool   string `json:"tool"`
	Prompt string `json:"prompt"`
}) []allowedPrompt {
	out := make([]allowedPrompt, 0, len(items))
	for _, item := range items {
		toolName := strings.ToLower(strings.TrimSpace(item.Tool))
		if toolName != "bash" && toolName != "powershell" {
			continue
		}
		prompt := strings.TrimSpace(item.Prompt)
		if prompt == "" {
			continue
		}
		out = append(out, allowedPrompt{Tool: toolName, Prompt: prompt})
	}
	return out
}

func renderPlanSteps(existing string, prompts []allowedPrompt) string {
	lines := make([]string, 0, len(prompts))
	for i, item := range prompts {
		lines = append(lines, fmt.Sprintf("%d. [%s] %s", i+1, item.Tool, item.Prompt))
	}
	body := strings.TrimSpace(existing)
	if body == "" {
		return strings.Join(lines, "\n")
	}
	return body + "\n\n" + strings.Join(lines, "\n")
}
