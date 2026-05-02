package query

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/koreaf16/argus/internal/state"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/taskplaninit"
	"github.com/koreaf16/argus/internal/types"
)

// ValidateWorkflowStep 은 도구 호출이 현재 워크플로우 단계에서 허용되는지 검증합니다.
func ValidateWorkflowStep(appState *state.AppState, toolName string, input json.RawMessage, isReadOnly bool) (bool, string) {
	if blocked, reason := pendingWorkflowInitBlocks(appState, toolName, isReadOnly); blocked {
		return true, reason
	}
	if blocked, reason := workflowPhaseBlocks(appState, toolName, input, isReadOnly); blocked {
		return true, reason
	}
	return false, ""
}

func workflowPhaseBlocks(appState *state.AppState, toolName string, input json.RawMessage, isReadOnly bool) (bool, string) {
	if appState == nil {
		return false, ""
	}
	card := appState.WorkflowCard()
	if card == nil {
		return false, ""
	}

	currentPhase := strings.ToLower(card.Phase)
	canonicalName := tool.CanonicalName(toolName)

	if appState.InPlanMode() && currentPhase == state.WorkflowPhasePlan && canonicalName == "todowrite" {
		return true, "Plan mode is active. Do not call TodoWrite while drafting the plan; call exit_plan_mode with the completed plan, or ask_user if approval/clarification is needed."
	}

	if canonicalName == "todowrite" && currentPhase == state.WorkflowPhasePlan && todoWriteAdvancesToExecute(input) {
		if !appState.WorkflowApproved() {
			return true, "Checkpoint failed: You are trying to move from 'plan' to 'execute' without explicit user approval. You MUST use the 'ask_user' tool to confirm the critical parameters (like DB Edition, Path, User) before proceeding."
		}
		if blocked, reason := requireWorkspaceBindingForExecute(card); blocked {
			return true, reason
		}
	}

	allowed := taskplaninit.PhaseAllowedTools(currentPhase)
	if allowed == nil {
		return false, ""
	}
	if !isToolInList(canonicalName, allowed) {
		return true, fmt.Sprintf("workflow phase=%s only allows: %s. Use TodoWrite to mark the current phase completed and the next phase in_progress before calling other tools.", currentPhase, strings.Join(allowed, ", "))
	}

	return false, ""
}

func pendingWorkflowInitBlocks(appState *state.AppState, toolName string, isReadOnly bool) (bool, string) {
	if appState == nil {
		return false, ""
	}

	// [해결] 이미 워크플로우 카드가 존재한다면 (세션 재개 포함) 초기화 차단을 하지 않음
	if appState.WorkflowCard() != nil {
		return false, ""
	}

	if !appState.PendingWorkflowInit() {
		return false, ""
	}

	// task_plan_init 자체는 통과
	if tool.CanonicalName(toolName) == taskplaninit.ToolName {
		return false, ""
	}

	return true, "Multi-step task detected. Call task_plan_init first to register a workflow checklist, then proceed phase-by-phase."
}

// triggerWorkflowHeuristic 은 빌드 호환성을 위해 유지합니다.
// 더 이상 자동으로 워크플로우를 강제하지 않도록 항상 false를 반환합니다.
func triggerWorkflowHeuristic(text string) bool {
	return false
}

func isToolInList(name string, list []string) bool {
	n := tool.CanonicalName(name)
	for _, item := range list {
		if tool.CanonicalName(item) == n {
			return true
		}
	}
	return false
}

// requireWorkspaceBindingForExecute 는 install/migration/tuning/integration 카테고리 작업이
// plan→execute 로 넘어갈 때 워크스페이스가 바인딩되어 있는지 검증한다. 멀티세션 가정 보호:
// sudo/dnf 같은 OS-specific 명령을 plan 단계에서 미리 박지 않으려면, 어떤 워크스페이스에
// 실행할지를 execute 진입 시점에 명시적으로 확정해야 한다.
func requireWorkspaceBindingForExecute(card *state.WorkflowCard) (bool, string) {
	if card == nil {
		return false, ""
	}
	switch card.Category {
	case state.WorkflowCategoryInstall, state.WorkflowCategoryMigration,
		state.WorkflowCategoryTuning, state.WorkflowCategoryIntegration:
	default:
		return false, ""
	}
	if len(card.WorkspaceRoles) == 0 && len(card.WorkspaceRoleProfiles) == 0 {
		return true, "Checkpoint failed: cannot enter 'execute' phase without a target workspace. Re-call task_plan_init with workspace_roles (e.g. {\"target_postgres\":\"sandbox-server\"}) so each step can be translated into the correct OS-specific commands."
	}
	return false, ""
}

func todoWriteAdvancesToExecute(input json.RawMessage) bool {
	var req struct {
		Phase string           `json:"phase"`
		Todos []types.TodoItem `json:"todos"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(req.Phase), state.WorkflowPhaseExecute) {
		return true
	}
	// req.Phase 가 비어 있어도 in_progress todo 가 execute 단계를 가리키면 전환으로 간주.
	// 이 폴백이 없으면 LLM 이 phase 필드를 누락한 채로 execute todo 를 in_progress 로 표시할 때
	// 승인 게이트가 우회되거나 (todowrite.go 폴백 추론 결과) 데드락이 발생한다.
	return strings.EqualFold(taskplaninit.InferPhaseFromTodos(req.Todos), state.WorkflowPhaseExecute)
}
