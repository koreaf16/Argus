package query

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/state"
	tool "github.com/koreaf16/argus/internal/tools"
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

func buildEvidencePlan(userText string, appState *state.AppState) evidencePlan {
	plan := evidencePlan{Required: make(map[string]bool)}
	if isSimpleGreeting(userText) {
		return plan
	}
	text := normalizeIntentText(userText)
	add := func(category string, reason string) {
		plan.Required[category] = true
		if reason != "" {
			plan.Reasons = append(plan.Reasons, reason)
		}
	}

	if appState != nil {
		if appState.PendingWorkflowInit() {
			add(tool.EvidencePlanning, "multi-step workflow initialization is pending")
		}
		if card := appState.WorkflowCard(); card != nil {
			switch strings.ToLower(strings.TrimSpace(card.Phase)) {
			case state.WorkflowPhaseDiscover:
				add(tool.EvidenceServerState, "discover phase needs environment state")
			case state.WorkflowPhaseResearch:
				add(tool.EvidenceExternalFresh, "research phase needs fresh external evidence")
			case state.WorkflowPhaseInterview:
				add(tool.EvidenceUserDecision, "interview phase needs user decisions")
			case state.WorkflowPhasePlan:
				add(tool.EvidencePlanning, "plan phase needs planning tools")
			case state.WorkflowPhaseExecute:
				add(tool.EvidenceMutation, "execute phase needs action tools")
			case state.WorkflowPhaseVerify:
				add(tool.EvidenceLocalCode, "verify phase needs concrete checks")
				add(tool.EvidenceServerState, "verify phase may need service state")
			}
		}
	}

	preferredDomains := detectPreferredDomains(text)
	if isLikelyExternalKnowledgeRequest(text, preferredDomains) || hasExplicitFreshExternalSignal(text, preferredDomains) {
		add(tool.EvidenceExternalFresh, "request depends on current or external facts")
	}
	if isLocalCodeEvidenceRequest(text) {
		add(tool.EvidenceLocalCode, "request depends on repository or file evidence")
	}
	if isServerEvidenceRequest(text) || isLocalOperationalInspectionRequest(text) {
		add(tool.EvidenceServerState, "request depends on server or runtime state")
	}
	if isUserDecisionEvidenceRequest(text) {
		add(tool.EvidenceUserDecision, "request needs an explicit user decision")
	}
	if isPlanningEvidenceRequest(text) {
		add(tool.EvidencePlanning, "request asks for planning")
	}
	if isMutationEvidenceRequest(text) {
		add(tool.EvidenceMutation, "request asks Argus to change or execute something")
	}
	return plan
}

func isLocalCodeEvidenceRequest(text string) bool {
	return hasAnyTerm(text, []string{
		"repo", "repository", "code", "file", "project", "workspace", "build",
		"test", "compile", "lint", "bug", "error", "stack trace", "failing",
		"function", "class", "package", "module", "go test", "npm run",
		"\ud504\ub85c\uc81d\ud2b8", "\ucf54\ub4dc", "\ud30c\uc77c", "\ube4c\ub4dc",
		"\ud14c\uc2a4\ud2b8", "\uc5d0\ub7ec", "\ubc84\uadf8",
	})
}

func isServerEvidenceRequest(text string) bool {
	return hasAnyTerm(text, []string{
		"server", "ssh", "remote", "host", "service", "daemon", "systemctl",
		"journalctl", "docker", "kubectl", "kubernetes", "nginx", "postgres",
		"postgresql", "mysql", "oracle", "redis", "deploy", "install",
		"\uc11c\ubc84", "\uc6d0\uaca9", "\uc11c\ube44\uc2a4", "\uc811\uc18d",
		"\uc124\uce58", "\ubc30\ud3ec",
	})
}

func isUserDecisionEvidenceRequest(text string) bool {
	return hasAnyTerm(text, []string{
		"ask me", "confirm with me", "choose", "which option", "user approval",
		"decision", "preference", "credential", "password", "api key",
		"\ubb3c\uc5b4", "\ud655\uc778\ubc1b", "\uc120\ud0dd", "\uc2b9\uc778",
	})
}

func isPlanningEvidenceRequest(text string) bool {
	return hasAnyTerm(text, []string{
		"plan", "proposal", "design", "architecture", "approach", "strategy",
		"roadmap", "steps", "checklist",
		"\ud50c\ub79c", "\uacc4\ud68d", "\uc124\uacc4", "\uc808\ucc28",
	})
}

func isMutationEvidenceRequest(text string) bool {
	return hasAnyTerm(text, []string{
		"implement", "fix", "change", "edit", "write", "create", "delete",
		"remove", "update", "upgrade", "install", "deploy", "migrate", "run",
		"execute", "apply", "start", "stop", "restart",
		"\uad6c\ud604", "\uc218\uc815", "\uace0\uccd0", "\uc791\uc131",
		"\uc0ad\uc81c", "\uc124\uce58", "\ubc30\ud3ec", "\uc2e4\ud589",
	})
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

func evidenceBlocksPrematureMutation(call llm.ToolUseStart, plan evidencePlan, st evidenceState, registry *tool.Registry) (bool, string) {
	readOnly := evidenceToolCallReadOnly(call, registry)
	// 읽기 전용(조회성) 명령어는 증거 수집 여부와 상관없이 항상 허용합니다.
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
	sort.Strings(missing)
	return true, fmt.Sprintf("mutation tool %q is blocked until required evidence is collected: %s. Gather evidence first or call tool_search if the needed evidence tool is not exposed.", call.Name, strings.Join(missing, ", "))
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

func evidenceToolCallReadOnly(call llm.ToolUseStart, registry *tool.Registry) bool {
	canonical := tool.CanonicalName(call.Name)
	switch canonical {
	case "bash":
		return tool.IsReadOnlyShellCommand(extractToolInputString(call.Input, "command"), "bash")
	case "powershell":
		return tool.IsReadOnlyShellCommand(extractToolInputString(call.Input, "command"), "powershell")
	}
	if registry == nil {
		return true
	}
	if impl, ok := registry.Lookup(call.Name); ok {
		return impl.IsReadOnly()
	}
	return true
}

func hiddenToolCallMessage(name string) string {
	return fmt.Sprintf("tool %q is not available for the current evidence-gated step. Use tool_search or gather the required evidence with the exposed tools before calling it.", name)
}

func (st *evidenceState) ObserveToolResult(call llm.ToolUseStart, isErr bool, registry *tool.Registry) {
	if isErr {
		return
	}
	readOnly := evidenceToolCallReadOnly(call, registry)
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
	missing := missingEvidenceCategories(plan, st)
	sort.Strings(missing)
	var sb strings.Builder
	sb.WriteString("Do not finalize your answer yet.\n")
	sb.WriteString("Resume directly. You need concrete evidence before answering or executing further.\n")
	sb.WriteString("Missing evidence categories: ")
	sb.WriteString(strings.Join(missing, ", "))
	sb.WriteString(".\n")
	sb.WriteString("Use the exposed evidence tools now. If the needed tool is not exposed, call tool_search with the missing evidence category first, then use the returned tool in the next step.\n")
	sb.WriteString("Do not claim a fact, file state, server state, or completed change until a tool result supports it.")
	return sb.String()
}

func buildEvidenceFailureMessage(plan evidencePlan, st evidenceState) string {
	missing := missingEvidenceCategories(plan, st)
	sort.Strings(missing)
	if len(missing) == 0 {
		return "Evidence checks passed."
	}
	return fmt.Sprintf("Required evidence was not collected: %s. I am not providing an unsupported answer.", strings.Join(missing, ", "))
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
