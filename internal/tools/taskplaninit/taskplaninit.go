// Package taskplaninit — 다단계 작업 오케스트레이터 진입 도구.
//
// LLM 이 설치/마이그레이션/튜닝/연동 같은 다단계 작업을 인지했을 때 가장 먼저
// 호출하여 6단계 표준 체크리스트를 todostore 에 등록하고 AppState 에 워크플로우
// 카드를 설정한다. 이후 engine.go 의 phase 화이트리스트 게이트가 단계별
// 허용 도구를 강제한다.
package taskplaninit

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/koreaf16/argus/internal/state"
	"github.com/koreaf16/argus/internal/todostore"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
)

const ToolName = "task_plan_init"

type Tool struct{}

func New() *Tool { return &Tool{} }

func (t *Tool) Name() string { return ToolName }

func (t *Tool) Description(ctx tool.Context) string {
	return strings.TrimSpace(`Initialize a structured multi-step plan for complex tasks (install, migration, tuning, integration).
Call this FIRST when the user asks for a task that needs environment discovery, manual lookup, user clarification, planning, and execution.
Registers a 6-phase checklist (discover -> research -> interview -> plan -> execute -> verify) and activates phase-gated tool restrictions.

MULTI-SESSION DISCIPLINE (critical for install/migration/tuning/integration):
- Always bind a target workspace via 'workspace_roles' (e.g. {"target_postgres": "sandbox-server"}).
- After 'discover' phase determines OS / version / package manager, RE-CALL this tool with workspace_role_profiles
  populated (os_family, os_version, package_manager, architecture). The card supports incremental updates.
- 'plan' phase MUST express intent ("install PostgreSQL 18 server packages on target_postgres") — not literal commands.
  Do NOT bake 'sudo', 'dnf', 'apt-get', '/etc/...', or other OS-specific tokens into plan steps.
- Concrete commands are decided at 'execute' phase using the captured OS profile.`)
}

func (t *Tool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{
				"type":        "string",
				"description": "Short title of the task (e.g. 'Postgres 14 -> 17 migration').",
			},
			"category": map[string]any{
				"type":        "string",
				"enum":        []string{"install", "migration", "tuning", "integration", "other"},
				"description": "Task category.",
			},
			"user_goal": map[string]any{
				"type":        "string",
				"description": "What the user wants to achieve, in one sentence.",
			},
			"known_constraints": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Known constraints / requirements / risks.",
			},
			"workspace_roles": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
				"description":          "Optional role-to-workspace map (e.g. source_mysql=oracle-server, target_postgres=sandbox-server).",
			},
			"workspace_role_profiles": map[string]any{
				"type": "object",
				"additionalProperties": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"channel":          map[string]any{"type": "string"},
						"server":           map[string]any{"type": "string"},
						"default_user":     map[string]any{"type": "string"},
						"as_user":          map[string]any{"type": "string"},
						"privilege_method": map[string]any{"type": "string", "description": "sudo, su, doas, runas, or empty"},
						"allowed_tools":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"mutation_policy":  map[string]any{"type": "string", "description": "read_only, write, transfer, or unrestricted"},
						"os_family":        map[string]any{"type": "string", "description": "rhel, debian, suse, alpine, windows, macos, etc. Populate after discover phase."},
						"os_version":       map[string]any{"type": "string", "description": "Distribution version (e.g. 9.4, 22.04, 11). Populate after discover phase."},
						"package_manager":  map[string]any{"type": "string", "description": "dnf, yum, apt, zypper, apk, brew, choco, winget, etc. Populate after discover phase."},
						"architecture":     map[string]any{"type": "string", "description": "x86_64, aarch64, arm64, etc. Populate after discover phase."},
					},
				},
				"description": "Optional typed role profiles. Each role binds channel/server/user/privilege policy and (after discover) OS/package-manager profile.",
			},
			"needs_research": map[string]any{
				"type":        "boolean",
				"description": "Whether external manual / docs lookup is needed. If false, the 'research' phase is skipped.",
			},
			"needs_user_input": map[string]any{
				"type":        "boolean",
				"description": "Whether the user must clarify options (paths, credentials, choices). If false, the 'interview' phase is skipped.",
			},
		},
		"required": []string{"title", "category"},
	}
}

func (t *Tool) IsReadOnly() bool { return true }

func (t *Tool) IsConcurrencySafe(input json.RawMessage) bool { return false }

func (t *Tool) MaxResultSizeChars() int { return 8000 }

type input struct {
	Title                 string                                `json:"title"`
	Category              string                                `json:"category"`
	UserGoal              string                                `json:"user_goal,omitempty"`
	KnownConstraints      []string                              `json:"known_constraints,omitempty"`
	WorkspaceRoles        map[string]string                     `json:"workspace_roles,omitempty"`
	WorkspaceRoleProfiles map[string]state.WorkspaceRoleProfile `json:"workspace_role_profiles,omitempty"`
	NeedsResearch         *bool                                 `json:"needs_research,omitempty"`
	NeedsUserInput        *bool                                 `json:"needs_user_input,omitempty"`
}

func (t *Tool) Call(ctx tool.Context, raw json.RawMessage) (<-chan tool.ToolEvent, error) {
	events := make(chan tool.ToolEvent, 2)
	go func() {
		defer close(events)

		if ctx.State == nil {
			events <- tool.NewErrorEvent(fmt.Errorf("state is unavailable"))
			return
		}

		var in input
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &in); err != nil {
				events <- tool.NewErrorEvent(fmt.Errorf("invalid input: %w", err))
				return
			}
		}

		title := strings.TrimSpace(in.Title)
		if title == "" {
			events <- tool.NewErrorEvent(fmt.Errorf("title is required"))
			return
		}
		category := normalizeCategory(in.Category)

		needsResearch := true
		if in.NeedsResearch != nil {
			needsResearch = *in.NeedsResearch
		}
		needsUserInput := true
		if in.NeedsUserInput != nil {
			needsUserInput = *in.NeedsUserInput
		}

		phases := buildPhases(needsResearch, needsUserInput)

		// 단축 경로: research/interview 둘 다 불필요하고 카테고리가 'other'일 때
		// 곧장 execute 단계로 점프한다 (false positive 비용 절감).
		shortcut := !needsResearch && !needsUserInput && category == state.WorkflowCategoryOther
		startPhase := state.WorkflowPhaseDiscover
		if shortcut {
			startPhase = state.WorkflowPhaseExecute
		}

		workspaceRoles := normalizeWorkspaceRoles(in.WorkspaceRoles)
		roleProfiles := normalizeWorkspaceRoleProfiles(in.WorkspaceRoleProfiles, workspaceRoles)
		card := &state.WorkflowCard{
			Title:                 title,
			Category:              category,
			Phase:                 startPhase,
			Goal:                  strings.TrimSpace(in.UserGoal),
			Constraints:           append([]string(nil), in.KnownConstraints...),
			WorkspaceRoles:        workspaceRoles,
			WorkspaceRoleProfiles: roleProfiles,
			StartedAt:             time.Now(),
		}
		ctx.State.SetWorkflowCard(card)
		ctx.State.SetPendingWorkflowInit(false)

		// 표준 체크리스트를 todostore 에 등록한다 (todowrite 패턴 미러).
		todos := buildTodos(phases, startPhase)
		sessionID := todostore.SessionID(ctx.State.SessionID())
		stored := todostore.NormalizeForStorage(todos)
		if err := todostore.Save(sessionID, stored); err != nil {
			events <- tool.NewErrorEvent(fmt.Errorf("save todos: %w", err))
			return
		}
		ctx.State.SetTodos(sessionID, stored)

		payload, _ := json.Marshal(map[string]any{
			"message":                 "workflow card initialized",
			"title":                   title,
			"category":                category,
			"phase":                   startPhase,
			"phases":                  phases,
			"guide":                   nextPhaseGuide(startPhase),
			"workspace_roles":         card.WorkspaceRoles,
			"workspace_role_profiles": card.WorkspaceRoleProfiles,
		})
		events <- tool.NewOutputEvent(string(payload))
		events <- tool.NewDoneEvent()
	}()
	return events, nil
}

func normalizeWorkspaceRoles(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		trimmedKey := strings.TrimSpace(key)
		trimmedValue := strings.TrimSpace(value)
		if trimmedKey == "" || trimmedValue == "" {
			continue
		}
		out[trimmedKey] = trimmedValue
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeWorkspaceRoleProfiles(in map[string]state.WorkspaceRoleProfile, legacy map[string]string) map[string]state.WorkspaceRoleProfile {
	out := make(map[string]state.WorkspaceRoleProfile)
	for key, profile := range in {
		normalizedKey := strings.TrimSpace(key)
		if normalizedKey == "" {
			normalizedKey = strings.TrimSpace(profile.Role)
		}
		if normalizedKey == "" {
			continue
		}
		profile.Role = firstNonEmpty(strings.TrimSpace(profile.Role), normalizedKey)
		profile.Channel = normalizeRoleField(profile.Channel)
		profile.Server = strings.TrimSpace(profile.Server)
		profile.DefaultUser = strings.TrimSpace(profile.DefaultUser)
		profile.AsUser = strings.TrimSpace(profile.AsUser)
		profile.PrivilegeMethod = normalizeRoleField(profile.PrivilegeMethod)
		profile.MutationPolicy = normalizeMutationPolicy(profile.MutationPolicy, normalizedKey, profile.Channel)
		profile.AllowedTools = normalizeToolList(profile.AllowedTools)
		profile.OSFamily = normalizeRoleField(profile.OSFamily)
		profile.OSVersion = strings.TrimSpace(profile.OSVersion)
		profile.PackageManager = normalizeRoleField(profile.PackageManager)
		profile.Architecture = normalizeRoleField(profile.Architecture)
		out[normalizedKey] = profile
	}
	for role, server := range legacy {
		if _, ok := out[role]; ok {
			continue
		}
		channel := inferChannel(role)
		out[role] = state.WorkspaceRoleProfile{
			Role:           role,
			Channel:        channel,
			Server:         server,
			MutationPolicy: normalizeMutationPolicy("", role, channel),
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func inferChannel(role string) string {
	lower := strings.ToLower(strings.TrimSpace(role))
	switch {
	case strings.Contains(lower, "source"):
		return "source"
	case strings.Contains(lower, "target"):
		return "target"
	case strings.Contains(lower, "transfer"), strings.Contains(lower, "copy"):
		return "transfer"
	case strings.Contains(lower, "verify"), strings.Contains(lower, "check"):
		return "verify"
	default:
		return ""
	}
}

func normalizeMutationPolicy(policy, role, channel string) string {
	policy = normalizeRoleField(policy)
	switch policy {
	case "read_only", "write", "transfer", "unrestricted":
		return policy
	}
	switch normalizeRoleField(firstNonEmpty(channel, role)) {
	case "source", "verify":
		return "read_only"
	case "transfer":
		return "transfer"
	case "target":
		return "write"
	default:
		return ""
	}
}

func normalizeToolList(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, item := range in {
		toolName := normalizeRoleField(item)
		if toolName == "" || seen[toolName] {
			continue
		}
		seen[toolName] = true
		out = append(out, toolName)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeRoleField(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (t *Tool) CheckPermission(ctx tool.Context, _ json.RawMessage) (tool.PermissionResult, error) {
	return tool.DefaultAllowPermission(), nil
}

func normalizeCategory(c string) string {
	switch strings.ToLower(strings.TrimSpace(c)) {
	case state.WorkflowCategoryInstall:
		return state.WorkflowCategoryInstall
	case state.WorkflowCategoryMigration:
		return state.WorkflowCategoryMigration
	case state.WorkflowCategoryTuning:
		return state.WorkflowCategoryTuning
	case state.WorkflowCategoryIntegration:
		return state.WorkflowCategoryIntegration
	default:
		return state.WorkflowCategoryOther
	}
}

func buildPhases(needsResearch, needsUserInput bool) []string {
	out := []string{state.WorkflowPhaseDiscover}
	if needsResearch {
		out = append(out, state.WorkflowPhaseResearch)
	}
	if needsUserInput {
		out = append(out, state.WorkflowPhaseInterview)
	}
	out = append(out, state.WorkflowPhasePlan, state.WorkflowPhaseExecute, state.WorkflowPhaseVerify)
	return out
}

func buildTodos(phases []string, startPhase string) []types.TodoItem {
	out := make([]types.TodoItem, 0, len(phases))
	startedMarked := false
	for _, phase := range phases {
		item := types.TodoItem{
			Content:    phaseTodoContent(phase),
			ActiveForm: phaseTodoActiveForm(phase),
			Status:     types.TodoStatusPending,
		}
		if !startedMarked && phase == startPhase {
			item.Status = types.TodoStatusInProgress
			startedMarked = true
		}
		out = append(out, item)
	}
	return out
}

func phaseTodoContent(phase string) string {
	switch phase {
	case state.WorkflowPhaseDiscover:
		return "Discover environment (OS, versions, existing installs)"
	case state.WorkflowPhaseResearch:
		return "Research official manuals / release notes"
	case state.WorkflowPhaseInterview:
		return "Interview user for required options / credentials"
	case state.WorkflowPhasePlan:
		return "Draft step-by-step plan and request approval"
	case state.WorkflowPhaseExecute:
		return "Execute approved plan steps"
	case state.WorkflowPhaseVerify:
		return "Verify outcome and report"
	default:
		return phase
	}
}

func phaseTodoActiveForm(phase string) string {
	switch phase {
	case state.WorkflowPhaseDiscover:
		return "Discovering environment"
	case state.WorkflowPhaseResearch:
		return "Researching manuals"
	case state.WorkflowPhaseInterview:
		return "Interviewing user"
	case state.WorkflowPhasePlan:
		return "Drafting plan"
	case state.WorkflowPhaseExecute:
		return "Executing plan"
	case state.WorkflowPhaseVerify:
		return "Verifying outcome"
	default:
		return phase
	}
}

func nextPhaseGuide(phase string) string {
	if phase == state.WorkflowPhasePlan {
		return "Now in 'plan' phase. Call enter_plan_mode once, draft intent-level steps that name a workspace role per step, then call exit_plan_mode with the completed plan text. Do not keep calling TodoWrite while plan mode is active; exit_plan_mode is the next step before approval/execution."
	}
	switch phase {
	case state.WorkflowPhaseDiscover:
		return "Now in 'discover' phase. Use bash, powershell, server_metrics, server_inspect, glob, grep, file_read, TodoWrite to inspect the environment of EACH bound workspace role. Capture: OS family (rhel/debian/...), OS version, package manager (dnf/apt/...), architecture, and existing relevant installations. Once captured, RE-CALL task_plan_init with workspace_role_profiles populated (os_family, os_version, package_manager, architecture per role) so subsequent phases can target the right OS. Then advance via TodoWrite."
	case state.WorkflowPhaseResearch:
		return "Now in 'research' phase. Use webfetch, web_search, MCP tools (e.g. context7), TodoWrite. Confirm version-specific procedures FROM PRIMARY SOURCES, scoped to the OS family / package manager you captured in discover. Cite the manual sections you relied on."
	case state.WorkflowPhaseInterview:
		return "Now in 'interview' phase. Use ask_user (batched questions preferred) to collect required choices (paths, credentials, edition, ports). Always confirm 'which workspace will run this' if any role binding is missing. Advance via TodoWrite."
	case state.WorkflowPhasePlan:
		return "Now in 'plan' phase. Draft a step-by-step plan via enter_plan_mode. RULES: each step must reference a workspace role (e.g. 'On target_postgres: install PostgreSQL 18 server packages'). Do NOT bake in 'sudo', 'dnf', 'apt-get', or absolute paths — those are decided at execute time from the OS profile. Submit via exit_plan_mode for user approval, then advance the todo via TodoWrite (which requires user approval to move to execute)."
	case state.WorkflowPhaseExecute:
		return "Now in 'execute' phase. For each step, look up the role's OS profile (workspace_role_profiles) and translate intent into concrete commands (correct package manager, sudo/runas/empty privilege method, distro-correct paths). If a profile field is missing, run a quick discover query first instead of guessing."
	case state.WorkflowPhaseVerify:
		return "Now in 'verify' phase. Use bash, powershell, server_metrics, file_read against each role's workspace to verify the outcome (service running, version matches, port listening). Mark all todos completed and report."
	default:
		return ""
	}
}

// phaseSignals 는 todo 항목의 첫 단어(소문자)에서 phase 를 추론하기 위한 키 표.
// taskplaninit 가 buildTodos 로 생성한 표준 문구와, LLM 이 흔히 쓰는 변형형을 모두 포함한다.
var phaseSignals = map[string]string{
	"discover":     state.WorkflowPhaseDiscover,
	"discovering":  state.WorkflowPhaseDiscover,
	"research":     state.WorkflowPhaseResearch,
	"researching":  state.WorkflowPhaseResearch,
	"interview":    state.WorkflowPhaseInterview,
	"interviewing": state.WorkflowPhaseInterview,
	"draft":        state.WorkflowPhasePlan,
	"drafting":     state.WorkflowPhasePlan,
	"execute":      state.WorkflowPhaseExecute,
	"executing":    state.WorkflowPhaseExecute,
	"verify":       state.WorkflowPhaseVerify,
	"verifying":    state.WorkflowPhaseVerify,
}

// allPhases 는 (phase) 태그 매칭에 사용한다.
var allPhases = []string{
	state.WorkflowPhaseDiscover,
	state.WorkflowPhaseResearch,
	state.WorkflowPhaseInterview,
	state.WorkflowPhasePlan,
	state.WorkflowPhaseExecute,
	state.WorkflowPhaseVerify,
}

// InferPhaseFromTodos 는 in_progress 인 todo 항목으로부터 워크플로우 phase 를 추론한다.
// 추론 우선순위:
//  1. content/activeForm 안의 "(phase)" 태그 — LLM 이 자체적으로 붙이는 명시적 힌트.
//  2. activeForm 의 첫 단어 (예: "Executing plan" -> execute).
//  3. content 의 첫 단어 (예: "Execute approved plan" -> execute).
//
// 단서가 전혀 없으면 빈 문자열을 반환한다 (호출자는 phase 를 변경하지 않는다).
func InferPhaseFromTodos(todos []types.TodoItem) string {
	for _, item := range todos {
		if item.Status != types.TodoStatusInProgress {
			continue
		}
		if phase := matchPhaseFromText(item.ActiveForm); phase != "" {
			return phase
		}
		if phase := matchPhaseFromText(item.Content); phase != "" {
			return phase
		}
	}
	return ""
}

func matchPhaseFromText(text string) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return ""
	}
	for _, phase := range allPhases {
		if strings.Contains(lower, "("+phase+")") {
			return phase
		}
	}
	fields := strings.Fields(lower)
	if len(fields) == 0 {
		return ""
	}
	return phaseSignals[fields[0]]
}

// PhaseAllowedTools 는 phase 별 화이트리스트를 반환한다.
// engine.go 의 workflowPhaseBlocks 가 사용한다. execute phase 는 nil 을 반환하여
// 제한 없음을 표시한다 (plan-mode 규칙은 별도 적용).
func PhaseAllowedTools(phase string) []string {
	// 모든 단계에서 공통적으로 허용되는 읽기 전용/기본 도구
	common := []string{
		"web_search", "webfetch", "fileread", "fs_list", "glob", "grep",
		"todoread", "todowrite", "task_plan_init", "ask_user", "tool_search",
		"mcp", "list_mcp_resources", "read_mcp_resource",
	}

	switch phase {
	case state.WorkflowPhaseDiscover:
		return append(common, "bash", "powershell", "server_metrics", "server_inspect", "server_connect")
	case state.WorkflowPhaseResearch:
		return common
	case state.WorkflowPhaseInterview:
		return common
	case state.WorkflowPhasePlan:
		return append(common, "enter_plan_mode", "exit_plan_mode")
	case state.WorkflowPhaseVerify:
		return append(common, "bash", "powershell", "server_metrics")
	default:
		return nil
	}
}
