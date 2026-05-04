package query

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/state"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/utils/permissions"
)

type evidencePlan struct {
	Required map[string]bool
	Reasons  []string
}

type evidenceState struct {
	LocalCodeSeen       bool
	ExternalFreshSeen   bool
	ServerStateSeen     bool
	UserDecisionSeen    bool
	MutationSeen        bool
	PlanningSeen        bool
	ForcedContinuations int
}

func evidenceToolExposureEnabled(cfg Config, registry *tool.Registry) bool {
	if !cfg.EvidenceToolExposure || registry == nil {
		return false
	}
	_, ok := registry.Lookup("tool_search")
	return ok
}

// buildEvidencePlan은 키워드 매칭 없이 항상 빈 플랜을 반환합니다.
// EvidenceToolExposure=false이므로 이 함수는 호출되지 않습니다.
func buildEvidencePlan(_ string, _ *state.AppState) evidencePlan {
	return evidencePlan{Required: make(map[string]bool)}
}

func filterToolSpecsForEvidence(specs []llm.ToolSpec, registry *tool.Registry, plan evidencePlan, selected map[string]bool) []llm.ToolSpec {
	if len(specs) == 0 {
		return specs
	}
	out := make([]llm.ToolSpec, 0, len(specs))
	for _, spec := range specs {
		if evidenceAllowsTool(spec.Name, registry, plan, selected) {
			out = append(out, spec)
		}
	}
	return out
}

func evidenceAllowsTool(name string, registry *tool.Registry, plan evidencePlan, selected map[string]bool) bool {
	canonical := tool.CanonicalName(name)
	if isCoreEvidenceTool(canonical) {
		return true
	}
	if selected != nil && selected[canonical] {
		return true
	}
	if len(plan.Required) == 0 {
		return false
	}
	readOnly := true
	if registry != nil {
		if impl, ok := registry.Lookup(name); ok {
			readOnly = impl.IsReadOnly()
		}
	}
	for _, category := range tool.ToolEvidenceCategories(name, readOnly) {
		if plan.Required[category] {
			return true
		}
	}
	return false
}

func isCoreEvidenceTool(canonical string) bool {
	switch canonical {
	case "ask_user", "tool_search", "task_plan_init":
		return true
	default:
		return false
	}
}

func toolSpecNameSet(specs []llm.ToolSpec) map[string]bool {
	out := make(map[string]bool, len(specs))
	for _, spec := range specs {
		out[tool.CanonicalName(spec.Name)] = true
	}
	return out
}

func isToolExposed(name string, exposed map[string]bool) bool {
	if exposed == nil {
		return true
	}
	return exposed[tool.CanonicalName(name)]
}

func evidenceBlocksPrematureMutation(
	ctx context.Context,
	call llm.ToolUseStart,
	plan evidencePlan,
	st evidenceState,
	registry *tool.Registry,
	subQuery permissions.SubQueryFunc,
	search permissions.SearchFunc,
	fetch permissions.FetchFunc,
) (bool, string) {
	readOnly := evidenceToolCallReadOnly(ctx, call, registry, subQuery, search, fetch)
	if readOnly {
		return false, ""
	}
	if !tool.ToolHasEvidenceCategory(call.Name, readOnly, tool.EvidenceMutation) {
		return false, ""
	}
	missing := missingEvidenceCategories(evidencePrerequisitePlan(plan), st)
	if len(missing) == 0 {
		return false, ""
	}
	return true, ""
}

func hiddenToolCallMessage(name string) string {
	return "tool \"" + name + "\" is not available for the current evidence-gated step. Use tool_search or gather the required evidence with the exposed tools before calling it."
}

func (st *evidenceState) ObserveToolResult(
	ctx context.Context,
	call llm.ToolUseStart,
	isErr bool,
	registry *tool.Registry,
	subQuery permissions.SubQueryFunc,
	search permissions.SearchFunc,
	fetch permissions.FetchFunc,
) {
	if isErr {
		return
	}
	readOnly := evidenceToolCallReadOnly(ctx, call, registry, subQuery, search, fetch)
	for _, category := range tool.ToolEvidenceCategories(call.Name, readOnly) {
		switch category {
		case tool.EvidenceLocalCode:
			st.LocalCodeSeen = true
		case tool.EvidenceExternalFresh:
			st.ExternalFreshSeen = true
		case tool.EvidenceServerState:
			st.ServerStateSeen = true
		case tool.EvidenceUserDecision:
			st.UserDecisionSeen = true
		case tool.EvidenceMutation:
			st.MutationSeen = true
		case tool.EvidencePlanning:
			st.PlanningSeen = true
		}
	}
}

func shouldBufferEvidenceAssistantText(plan evidencePlan, st evidenceState) bool {
	return len(missingEvidenceCategories(plan, st)) > 0
}

func shouldForceEvidenceContinuation(plan evidencePlan, st evidenceState, cfg Config) bool {
	if !shouldBufferEvidenceAssistantText(plan, st) {
		return false
	}
	return st.ForcedContinuations < cfg.MaxForcedContinuations
}

func buildEvidenceFollowUpPrompt(plan evidencePlan, st evidenceState) string {
	_ = plan
	_ = st
	return "Do not finalize your answer yet. Resume directly with the required evidence tools."
}

func buildEvidenceFailureMessage(plan evidencePlan, st evidenceState) string {
	missing := missingEvidenceCategories(plan, st)
	if len(missing) == 0 {
		return "Evidence checks passed."
	}
	return "Required evidence was not collected. I am not providing an unsupported answer."
}

func missingEvidenceCategories(plan evidencePlan, st evidenceState) []string {
	missing := make([]string, 0, len(plan.Required))
	for category := range plan.Required {
		switch category {
		case tool.EvidenceLocalCode:
			if !st.LocalCodeSeen {
				missing = append(missing, category)
			}
		case tool.EvidenceExternalFresh:
			if !st.ExternalFreshSeen {
				missing = append(missing, category)
			}
		case tool.EvidenceServerState:
			if !st.ServerStateSeen {
				missing = append(missing, category)
			}
		case tool.EvidenceUserDecision:
			if !st.UserDecisionSeen {
				missing = append(missing, category)
			}
		case tool.EvidenceMutation:
			if !st.MutationSeen {
				missing = append(missing, category)
			}
		case tool.EvidencePlanning:
			if !st.PlanningSeen {
				missing = append(missing, category)
			}
		}
	}
	return missing
}

func prefillEvidenceFromHistory(ctx context.Context, st *evidenceState, messages []llm.Message, registry *tool.Registry) {
	const lookback = 20
	start := max(0, len(messages)-lookback)
	for _, msg := range messages[start:] {
		for _, block := range msg.Content {
			if block.Type != llm.ContentToolResult || block.IsError {
				continue
			}
			call := llm.ToolUseStart{ID: block.ToolUseID, Name: block.Name, Input: block.Input}
			st.ObserveToolResult(ctx, call, false, registry, nil, nil, nil)
		}
	}
}

func parseToolSearchResultNames(result string) []string {
	var payload struct {
		Candidates []struct {
			Name string `json:"name"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(result)), &payload); err != nil {
		return nil
	}
	out := make([]string, 0, len(payload.Candidates))
	for _, item := range payload.Candidates {
		name := strings.TrimSpace(item.Name)
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

func evidencePrerequisitePlan(plan evidencePlan) evidencePlan {
	out := evidencePlan{Required: make(map[string]bool)}
	for category := range plan.Required {
		if category == tool.EvidenceMutation {
			continue
		}
		out.Required[category] = true
	}
	return out
}

func evidenceToolCallReadOnly(
	ctx context.Context,
	call llm.ToolUseStart,
	registry *tool.Registry,
	subQuery permissions.SubQueryFunc,
	search permissions.SearchFunc,
	fetch permissions.FetchFunc,
) bool {
	canonical := tool.CanonicalName(call.Name)
	if canonical == "bash" || canonical == "powershell" {
		command := extractToolInputString(call.Input, "command")
		if command == "" {
			return false
		}
		if tool.IsReadOnlyShellCommand(command, canonical) {
			return true
		}
		if subQuery != nil {
			result := permissions.ClassifyBashCommand(ctx, command, permissions.NewDenialTrackingState(), subQuery, search, fetch)
			return result.Behavior == "allow"
		}
		return false
	}
	if registry == nil {
		return true
	}
	if impl, ok := registry.Lookup(call.Name); ok {
		return impl.IsReadOnly()
	}
	return true
}

// extractTraceServers는 도구 호출에서 서버 별칭을 추출합니다.
func extractTraceServers(toolName string, input []byte) []string {
	var raw map[string]any
	if err := json.Unmarshal(input, &raw); err != nil {
		return nil
	}
	seen := make(map[string]bool)
	out := make([]string, 0, 2)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	if v, ok := raw["server"].(string); ok {
		add(v)
	}
	canonical := tool.CanonicalName(toolName)
	if canonical == "server_copy" {
		for _, key := range []string{"src", "dst"} {
			if s, ok := raw[key].(string); ok {
				if i := strings.Index(s, ":"); i > 0 {
					add(s[:i])
				}
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
