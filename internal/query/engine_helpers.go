package query

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/koreaf16/argus/internal/constants"
	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/state"
	"github.com/koreaf16/argus/internal/tools/taskplaninit"
)

func buildPlannedStepToolCallInput(step PlannedStep, workingDir string) (json.RawMessage, error) {
	body := map[string]any{
		"command": strings.TrimSpace(step.Prompt),
	}
	if strings.TrimSpace(step.Server) != "" {
		body["server"] = strings.TrimSpace(step.Server)
	}
	if strings.TrimSpace(step.Role) != "" {
		body["role"] = strings.TrimSpace(step.Role)
	}
	if strings.TrimSpace(step.Channel) != "" {
		body["channel"] = strings.TrimSpace(step.Channel)
	}
	if strings.TrimSpace(step.AsUser) != "" {
		body["as_user"] = strings.TrimSpace(step.AsUser)
	}
	if strings.TrimSpace(step.PrivilegeMethod) != "" {
		body["privilege_method"] = strings.TrimSpace(step.PrivilegeMethod)
	}
	if strings.TrimSpace(workingDir) != "" {
		body["workdir"] = workingDir
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func activeModelContextWindow(appState *state.AppState) int {
	if appState == nil {
		return 0
	}
	return appState.ActiveModelContext()
}

func isSimpleGreeting(text string) bool {
	s := strings.ToLower(strings.TrimSpace(text))
	if len(s) == 0 || len([]rune(s)) > 20 {
		return false
	}
	greetings := []string{
		"하이", "안녕", "hi", "hello", "hey", "안녕하세", "반가워",
		"굿모닝", "굿애프터눈", "굿이브닝", "ㅎㅇ", "방가", "반갑습니",
		"좋은 아침", "좋은 하루", "요즘 어때", "뭐해", "누구니",
	}
	for _, g := range greetings {
		if strings.Contains(s, g) {
			return true
		}
	}
	return false
}

func filterToolSpecs(specs []llm.ToolSpec, appState *state.AppState) []llm.ToolSpec {
	if len(specs) == 0 || appState == nil {
		return specs
	}
	card := appState.WorkflowCard()
	if card == nil {
		if appState.PendingWorkflowInit() {
			return filterToolSpecsByAllowed(specs, []string{taskplaninit.ToolName})
		}
		return specs
	}
	phase := strings.ToLower(strings.TrimSpace(card.Phase))
	if phase == "" {
		phase = state.WorkflowPhaseDiscover
	}
	allowed := taskplaninit.PhaseAllowedTools(phase)
	if appState.InPlanMode() && strings.EqualFold(phase, state.WorkflowPhasePlan) {
		allowed = []string{"exit_plan_mode", "TodoRead", taskplaninit.ToolName, "ask_user", "ask_user_batch", "tool_search"}
	}
	if allowed == nil {
		return specs
	}
	return filterToolSpecsByAllowed(specs, allowed)
}

func filterToolSpecsByAllowed(specs []llm.ToolSpec, allowed []string) []llm.ToolSpec {
	if len(allowed) == 0 {
		return nil
	}
	out := make([]llm.ToolSpec, 0, len(specs))
	for _, spec := range specs {
		if isToolInList(spec.Name, allowed) {
			out = append(out, spec)
		}
	}
	return out
}

func parsePlanExecutionReady(toolName, result string, isErr bool) []PlannedStep {
	if isErr || !isExitPlanModeToolName(toolName) {
		return nil
	}
	var payload struct {
		AllowedPrompts []struct {
			Tool            string `json:"tool"`
			Prompt          string `json:"prompt"`
			Server          string `json:"server"`
			Role            string `json:"role"`
			Channel         string `json:"channel"`
			AsUser          string `json:"as_user"`
			PrivilegeMethod string `json:"privilege_method"`
		} `json:"allowed_prompts"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		return nil
	}
	out := make([]PlannedStep, 0, len(payload.AllowedPrompts))
	for _, item := range payload.AllowedPrompts {
		toolName := strings.ToLower(strings.TrimSpace(item.Tool))
		prompt := strings.TrimSpace(item.Prompt)
		if (toolName == "bash" || toolName == "powershell") && prompt != "" {
			out = append(out, PlannedStep{
				Tool:            toolName,
				Prompt:          prompt,
				Server:          strings.TrimSpace(item.Server),
				Role:            strings.TrimSpace(item.Role),
				Channel:         strings.TrimSpace(item.Channel),
				AsUser:          strings.TrimSpace(item.AsUser),
				PrivilegeMethod: strings.TrimSpace(item.PrivilegeMethod),
			})
		}
	}
	return out
}

func isExitPlanModeToolName(toolName string) bool {
	name := strings.TrimSpace(toolName)
	return strings.EqualFold(name, constants.ExitPlanModeToolName) ||
		strings.EqualFold(name, constants.LegacyExitPlanModeToolName)
}

func truncateLargeData(data any) any {
	const maxChars = 4096 // 8192바이트 대신 약 4000글자 기준으로 제한
	switch v := data.(type) {
	case string:
		runes := []rune(v)
		if len(runes) > maxChars {
			half := maxChars / 2
			return string(runes[:half]) + fmt.Sprintf("\n...[truncated %d characters]...\n", len(runes)-maxChars) + string(runes[len(runes)-half:])
		}
	case json.RawMessage:
		s := string(v)
		runes := []rune(s)
		if len(runes) > maxChars {
			half := maxChars / 2
			truncated := string(runes[:half]) + fmt.Sprintf("\n...[truncated %d characters]...\n", len(runes)-maxChars) + string(runes[len(runes)-half:])
			return json.RawMessage([]byte(truncated))
		}
		return v
	case []byte:
		s := string(v)
		runes := []rune(s)
		if len(runes) > maxChars {
			half := maxChars / 2
			truncated := string(runes[:half]) + fmt.Sprintf("\n...[truncated %d characters]...\n", len(runes)-maxChars) + string(runes[len(runes)-half:])
			return []byte(truncated)
		}
		return v
	case map[string]any:
		newMap := make(map[string]any)
		for k, val := range v {
			newMap[k] = truncateLargeData(val)
		}
		return newMap
	case []any:
		newSlice := make([]any, len(v))
		for i, val := range v {
			newSlice[i] = truncateLargeData(val)
		}
		return newSlice
	}
	return data
}

func isShellTool(toolName string) bool {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "bash", "powershell":
		return true
	default:
		return false
	}
}

func extractOutputTaskID(output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return ""
	}
	if v, ok := payload["OutputTaskID"].(string); ok {
		return strings.TrimSpace(v)
	}
	if v, ok := payload["output_task_id"].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func isPlanModeWriteException(toolName string) bool {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "todowrite", "exitplanmode", "exit_plan_mode":
		return true
	default:
		return false
	}
}

const repeatedToolCallLimit = 3

type repeatedToolCallGuard struct {
	lastSig string
	count   int
	limit   int
}

func newRepeatedToolCallGuard(limit int) repeatedToolCallGuard {
	if limit <= 0 {
		limit = repeatedToolCallLimit
	}
	return repeatedToolCallGuard{limit: limit}
}

func (g *repeatedToolCallGuard) Observe(calls []llm.ToolUseStart) (bool, int, string) {
	if len(calls) == 0 {
		g.lastSig = ""
		g.count = 0
		return false, 0, ""
	}
	sig := toolCallsSignature(calls)
	if sig == g.lastSig {
		g.count++
	} else {
		g.lastSig = sig
		g.count = 1
	}
	return g.count >= g.limit, g.count, sig
}

func toolCallsSignature(calls []llm.ToolUseStart) string {
	parts := make([]string, 0, len(calls))
	for _, call := range calls {
		parts = append(parts, toolCallSignature(call))
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}

func toolCallSignature(call llm.ToolUseStart) string {
	name := strings.ToLower(strings.TrimSpace(call.Name))
	input := strings.TrimSpace(string(call.Input))
	var payload any
	if len(call.Input) > 0 && json.Unmarshal(call.Input, &payload) == nil {
		if normalized, err := json.Marshal(payload); err == nil {
			input = string(normalized)
		}
	}
	return name + ":" + input
}

func summarizeToolCallSignature(sig string) string {
	sig = strings.TrimSpace(sig)
	if len(sig) <= 240 {
		return sig
	}
	return sig[:240] + "...(truncated)"
}
