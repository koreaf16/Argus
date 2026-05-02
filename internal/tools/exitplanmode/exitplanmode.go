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
	return "Submit the completed plan and exit plan mode. In workflow plan phase, call this after drafting the plan; do not repeat TodoWrite while the plan todo is still in progress."
}

func (t *ExitPlanModeTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"plan": map[string]any{
				"type":        "string",
				"description": "Optional completed plan text to persist and present for approval before leaving plan mode.",
			},
			"planText": map[string]any{
				"type":        "string",
				"description": "Alias for plan.",
			},
			"allowedPrompts": map[string]any{
				"type":        "array",
				"description": "Optional executable shell steps derived from the plan. Use only bash or powershell, with workspace role/server metadata when relevant.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"tool":             map[string]any{"type": "string"},
						"prompt":           map[string]any{"type": "string"},
						"server":           map[string]any{"type": "string"},
						"role":             map[string]any{"type": "string"},
						"channel":          map[string]any{"type": "string"},
						"as_user":          map[string]any{"type": "string"},
						"privilege_method": map[string]any{"type": "string"},
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

func (t *ExitPlanModeTool) IsConcurrencySafe(input json.RawMessage) bool {
	return false
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
			Plan           string `json:"plan"`
			PlanText       string `json:"planText"`
			AllowedPrompts []struct {
				Tool            string `json:"tool"`
				Prompt          string `json:"prompt"`
				Server          string `json:"server"`
				Role            string `json:"role"`
				Channel         string `json:"channel"`
				AsUser          string `json:"as_user"`
				PrivilegeMethod string `json:"privilege_method"`
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
		if submittedPlan := strings.TrimSpace(firstNonEmpty(req.Plan, req.PlanText)); submittedPlan != "" {
			planText = mergeSubmittedPlan(planText, submittedPlan)
			planPath, err = toolfs.WritePlan(sessionID, planText)
			if err != nil {
				events <- tool.NewErrorEvent(err)
				return
			}
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
	if ctx.State != nil && ctx.State.InYoloMode() {
		return tool.DefaultAllowPermission(), nil
	}
	return tool.PermissionResult{
		Behavior: types.BehaviorAsk,
		Message:  "Present plan for approval",
	}, nil
}

type allowedPrompt struct {
	Tool            string `json:"tool"`
	Prompt          string `json:"prompt"`
	Server          string `json:"server,omitempty"`
	Role            string `json:"role,omitempty"`
	Channel         string `json:"channel,omitempty"`
	AsUser          string `json:"as_user,omitempty"`
	PrivilegeMethod string `json:"privilege_method,omitempty"`
}

func normalizeAllowedPrompts(items []struct {
	Tool            string `json:"tool"`
	Prompt          string `json:"prompt"`
	Server          string `json:"server"`
	Role            string `json:"role"`
	Channel         string `json:"channel"`
	AsUser          string `json:"as_user"`
	PrivilegeMethod string `json:"privilege_method"`
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
		out = append(out, allowedPrompt{
			Tool:            toolName,
			Prompt:          prompt,
			Server:          strings.TrimSpace(item.Server),
			Role:            strings.TrimSpace(item.Role),
			Channel:         strings.TrimSpace(item.Channel),
			AsUser:          strings.TrimSpace(item.AsUser),
			PrivilegeMethod: strings.TrimSpace(item.PrivilegeMethod),
		})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func mergeSubmittedPlan(existing, submitted string) string {
	existing = strings.TrimSpace(existing)
	submitted = strings.TrimSpace(submitted)
	if existing == "" {
		return submitted
	}
	if submitted == "" || strings.Contains(existing, submitted) {
		return existing
	}
	return existing + "\n\n" + submitted
}

func renderPlanSteps(existing string, prompts []allowedPrompt) string {
	lines := make([]string, 0, len(prompts))
	for i, item := range prompts {
		label := item.Tool
		var tags []string
		if strings.TrimSpace(item.Server) != "" {
			tags = append(tags, strings.TrimSpace(item.Server))
		}
		if strings.TrimSpace(item.Role) != "" {
			tags = append(tags, "role="+strings.TrimSpace(item.Role))
		}
		if strings.TrimSpace(item.Channel) != "" {
			tags = append(tags, "channel="+strings.TrimSpace(item.Channel))
		}
		if strings.TrimSpace(item.AsUser) != "" {
			tags = append(tags, "as="+strings.TrimSpace(item.AsUser))
		}
		if strings.TrimSpace(item.PrivilegeMethod) != "" {
			tags = append(tags, "via="+strings.TrimSpace(item.PrivilegeMethod))
		}
		if len(tags) > 0 {
			label = fmt.Sprintf("%s@%s", item.Tool, strings.Join(tags, ","))
		}
		lines = append(lines, fmt.Sprintf("%d. [%s] %s", i+1, label, item.Prompt))
	}
	body := strings.TrimSpace(existing)
	if body == "" {
		return strings.Join(lines, "\n")
	}
	return body + "\n\n" + strings.Join(lines, "\n")
}
