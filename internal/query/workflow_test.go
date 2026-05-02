package query

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/state"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/taskplaninit"
	"github.com/koreaf16/argus/internal/types"
)

func TestTriggerWorkflowHeuristic(t *testing.T) {
	cases := map[string]bool{
		"Postgres 14에서 17로 마이그레이션해줘": true,
		"redis 설치해줘":                       true,
		"please install nginx with TLS":    true,
		"upgrade kubernetes to 1.30":       true,
		"can you set up a CI pipeline":     true,
		"configure DNS for the new domain": true,
		"ls":                               false,
		"hello":                            false,
		"show me the current disk usage":   false,
		"":                                 false,
	}
	for input, want := range cases {
		got := triggerWorkflowHeuristic(input)
		if got != want {
			t.Errorf("triggerWorkflowHeuristic(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestWorkflowPhaseBlocksAllowsExecute(t *testing.T) {
	st := state.NewAppState()
	st.SetWorkflowCard(&state.WorkflowCard{
		Title:    "x",
		Category: state.WorkflowCategoryMigration,
		Phase:    state.WorkflowPhaseExecute,
	})
	if blocked, _ := workflowPhaseBlocks(st, "bash", nil, false); blocked {
		t.Fatal("execute phase should not block any tool")
	}
}

func TestWorkflowPhaseBlocksRejectsBashInResearch(t *testing.T) {
	st := state.NewAppState()
	st.SetWorkflowCard(&state.WorkflowCard{
		Title:    "x",
		Category: state.WorkflowCategoryMigration,
		Phase:    state.WorkflowPhaseResearch,
	})
	blocked, reason := workflowPhaseBlocks(st, "bash", nil, false)
	if !blocked {
		t.Fatal("research phase must block bash")
	}
	if !strings.Contains(reason, "research") {
		t.Fatalf("reason should mention phase: %s", reason)
	}
}

func TestWorkflowPhaseBlocksAllowsTaskPlanInitInAnyPhase(t *testing.T) {
	st := state.NewAppState()
	st.SetWorkflowCard(&state.WorkflowCard{
		Title:    "x",
		Category: state.WorkflowCategoryMigration,
		Phase:    state.WorkflowPhaseInterview,
	})
	if blocked, _ := workflowPhaseBlocks(st, taskplaninit.ToolName, nil, false); blocked {
		t.Fatal("task_plan_init must always be allowed")
	}
}

func TestWorkflowPhaseBlocksAllowsWebFetchAliasesInResearch(t *testing.T) {
	st := state.NewAppState()
	st.SetWorkflowCard(&state.WorkflowCard{
		Title:    "x",
		Category: state.WorkflowCategoryMigration,
		Phase:    state.WorkflowPhaseResearch,
	})
	for _, name := range []string{"webfetch", "web_fetch"} {
		if blocked, reason := workflowPhaseBlocks(st, name, nil, false); blocked {
			t.Fatalf("%s should be allowed in research: %s", name, reason)
		}
	}
}

func TestFilterToolSpecsLimitsPendingWorkflowInit(t *testing.T) {
	st := state.NewAppState()
	st.SetPendingWorkflowInit(true)
	specs := []llm.ToolSpec{
		{Name: "bash"},
		{Name: taskplaninit.ToolName},
		{Name: "web_search"},
	}

	got := filterToolSpecs(specs, st)
	if len(got) != 1 || got[0].Name != taskplaninit.ToolName {
		t.Fatalf("expected only task_plan_init, got %+v", got)
	}
}

func TestFilterToolSpecsLimitsWorkflowPhase(t *testing.T) {
	st := state.NewAppState()
	st.SetWorkflowCard(&state.WorkflowCard{
		Title:    "x",
		Category: state.WorkflowCategoryMigration,
		Phase:    state.WorkflowPhaseResearch,
	})
	specs := []llm.ToolSpec{
		{Name: "bash"},
		{Name: "web_search"},
		{Name: "webfetch"},
		{Name: "TodoWrite"},
	}

	got := filterToolSpecs(specs, st)
	if hasToolSpec(got, "bash") {
		t.Fatalf("bash should be hidden in research phase: %+v", got)
	}
	for _, name := range []string{"web_search", "webfetch", "TodoWrite"} {
		if !hasToolSpec(got, name) {
			t.Fatalf("%s should remain visible in research phase: %+v", name, got)
		}
	}
}

func TestFilterToolSpecsHidesTodoWriteDuringActivePlanMode(t *testing.T) {
	st := state.NewAppState()
	st.SetWorkflowCard(&state.WorkflowCard{
		Title:    "x",
		Category: state.WorkflowCategoryInstall,
		Phase:    state.WorkflowPhasePlan,
	})
	st.SetMode("plan")
	st.SetPermissionMode(types.PermissionModePlan)
	if !st.InPlanMode() {
		t.Fatal("test setup failed: state should be in plan mode")
	}
	if st.WorkflowCard() == nil {
		t.Fatal("test setup failed: workflow card should be set")
	}
	if phase := strings.ToLower(strings.TrimSpace(st.WorkflowCard().Phase)); phase != state.WorkflowPhasePlan {
		t.Fatalf("test setup failed: phase=%q", phase)
	}

	specs := []llm.ToolSpec{
		{Name: "enter_plan_mode"},
		{Name: "exit_plan_mode"},
		{Name: "TodoWrite"},
		{Name: "TodoRead"},
	}

	got := filterToolSpecs(specs, st)
	if hasToolSpec(got, "TodoWrite") {
		t.Fatalf("TodoWrite should be hidden while plan mode is active: %+v", got)
	}
	if hasToolSpec(got, "enter_plan_mode") {
		t.Fatalf("enter_plan_mode should be hidden after plan mode is already active: %+v", got)
	}
	if !hasToolSpec(got, "exit_plan_mode") {
		t.Fatalf("exit_plan_mode must remain available while plan mode is active: %+v", got)
	}
}

func hasToolSpec(specs []llm.ToolSpec, name string) bool {
	for _, spec := range specs {
		if spec.Name == name {
			return true
		}
	}
	return false
}

func TestWorkflowPhaseBlocksPlanTodoWriteOnlyRequiresApprovalForExecute(t *testing.T) {
	st := state.NewAppState()
	st.SetWorkflowCard(&state.WorkflowCard{
		Title:          "x",
		Category:       state.WorkflowCategoryMigration,
		Phase:          state.WorkflowPhasePlan,
		WorkspaceRoles: map[string]string{"target_postgres": "sandbox-server"},
	})

	planInput := json.RawMessage(`{"todos":[{"content":"plan","status":"in_progress","activeForm":"planning"}],"phase":"plan"}`)
	if blocked, reason := workflowPhaseBlocks(st, "TodoWrite", planInput, false); blocked {
		t.Fatalf("plan TodoWrite should be allowed without approval: %s", reason)
	}

	executeInput := json.RawMessage(`{"todos":[{"content":"execute","status":"in_progress","activeForm":"running"}],"phase":"execute"}`)
	if blocked, _ := workflowPhaseBlocks(st, "TodoWrite", executeInput, false); !blocked {
		t.Fatal("execute transition should require approval")
	}

	st.SetWorkflowApproved(true)
	if blocked, reason := workflowPhaseBlocks(st, "TodoWrite", executeInput, false); blocked {
		t.Fatalf("approved execute transition should be allowed: %s", reason)
	}
}

// install/migration/tuning/integration 카테고리는 워크스페이스 바인딩 없이
// execute 단계로 진입할 수 없어야 한다 (multi-session sudo 사전정의 방지 가드).
func TestWorkflowPhaseBlocksTodoWriteWhilePlanModeActive(t *testing.T) {
	st := state.NewAppState()
	st.SetWorkflowCard(&state.WorkflowCard{
		Title:    "x",
		Category: state.WorkflowCategoryInstall,
		Phase:    state.WorkflowPhasePlan,
	})
	st.SetMode("plan")
	st.SetPermissionMode(types.PermissionModePlan)

	input := json.RawMessage(`{"phase":"plan","todos":[{"content":"Draft plan","status":"in_progress","activeForm":"Drafting plan"}]}`)
	blocked, reason := workflowPhaseBlocks(st, "TodoWrite", input, false)
	if !blocked {
		t.Fatal("TodoWrite should be blocked while plan mode is active")
	}
	if !strings.Contains(reason, "exit_plan_mode") {
		t.Fatalf("reason should direct model to exit_plan_mode, got: %s", reason)
	}
}

func TestWorkflowPhaseBlocksExecuteAdvanceWithoutWorkspaceBinding(t *testing.T) {
	st := state.NewAppState()
	st.SetWorkflowCard(&state.WorkflowCard{
		Title:    "x",
		Category: state.WorkflowCategoryInstall,
		Phase:    state.WorkflowPhasePlan,
	})
	st.SetWorkflowApproved(true) // 사용자 승인은 받았다고 가정

	executeInput := json.RawMessage(`{"phase":"execute","todos":[
		{"content":"Execute approved plan","status":"in_progress","activeForm":"Executing plan"}
	]}`)
	blocked, reason := workflowPhaseBlocks(st, "TodoWrite", executeInput, false)
	if !blocked {
		t.Fatal("install workflow without workspace_roles must not enter execute")
	}
	if !strings.Contains(reason, "task_plan_init") {
		t.Fatalf("reason should guide user to re-call task_plan_init: %s", reason)
	}
}

// category=other 는 워크스페이스 바인딩 가드를 적용하지 않는다 (단순 작업 우회 경로).
func TestWorkflowPhaseAllowsExecuteAdvanceForOtherCategoryWithoutBinding(t *testing.T) {
	st := state.NewAppState()
	st.SetWorkflowCard(&state.WorkflowCard{
		Title:    "x",
		Category: state.WorkflowCategoryOther,
		Phase:    state.WorkflowPhasePlan,
	})
	st.SetWorkflowApproved(true)

	executeInput := json.RawMessage(`{"phase":"execute","todos":[
		{"content":"Execute approved plan","status":"in_progress","activeForm":"Executing plan"}
	]}`)
	if blocked, reason := workflowPhaseBlocks(st, "TodoWrite", executeInput, false); blocked {
		t.Fatalf("category=other should bypass workspace binding requirement: %s", reason)
	}
}

// LLM 이 phase 필드를 누락한 채로 execute todo 를 in_progress 로 표시해도
// 승인 게이트가 작동해야 한다 (todowrite 의 phase 추론 폴백과 짝을 이루는 회귀 가드).
func TestWorkflowPhaseBlocksExecuteAdvanceFromTodosWithoutPhaseField(t *testing.T) {
	st := state.NewAppState()
	st.SetWorkflowCard(&state.WorkflowCard{
		Title:          "x",
		Category:       state.WorkflowCategoryInstall,
		Phase:          state.WorkflowPhasePlan,
		WorkspaceRoles: map[string]string{"target_postgres": "sandbox-server"},
	})

	// phase 필드 없음, in_progress todo 의 activeForm 이 "Executing plan" 인 케이스
	input := json.RawMessage(`{"todos":[
		{"content":"Execute approved plan","status":"in_progress","activeForm":"Executing plan"}
	]}`)
	if blocked, _ := workflowPhaseBlocks(st, "TodoWrite", input, false); !blocked {
		t.Fatal("execute transition inferred from in_progress todo must require approval")
	}

	st.SetWorkflowApproved(true)
	if blocked, reason := workflowPhaseBlocks(st, "TodoWrite", input, false); blocked {
		t.Fatalf("approved execute transition should pass, got: %s", reason)
	}
}

func TestPendingWorkflowInitBlocksMutatingTools(t *testing.T) {
	st := state.NewAppState()
	st.SetPendingWorkflowInit(true)

	// mutating tool: blocked
	blocked, reason := pendingWorkflowInitBlocks(st, "bash", false)
	if !blocked {
		t.Fatal("expected pending block for bash")
	}
	if !strings.Contains(reason, "task_plan_init") {
		t.Fatalf("reason should mention task_plan_init: %s", reason)
	}

	// read-only: also blocked until task_plan_init registers the workflow
	if blocked, _ := pendingWorkflowInitBlocks(st, "file_read", true); !blocked {
		t.Fatal("read-only tool should also be blocked")
	}

	// task_plan_init: passes
	if blocked, _ := pendingWorkflowInitBlocks(st, taskplaninit.ToolName, true); blocked {
		t.Fatal("task_plan_init must pass")
	}
}

func TestPendingWorkflowInitNoBlockWhenFlagOff(t *testing.T) {
	st := state.NewAppState()
	if blocked, _ := pendingWorkflowInitBlocks(st, "bash", false); blocked {
		t.Fatal("no flag = no block")
	}
}

// 통합: pending 플래그가 켜진 상태에서 invokeTool 이 mutating bash 호출을
// 차단하는지 확인.
func TestInvokeToolBlocksMutatingToolWhenPendingWorkflow(t *testing.T) {
	reg := tool.NewRegistry()
	fake := &fakeShellTool{
		name:       "bash",
		permission: types.BehaviorAllow,
	}
	if err := reg.Register(fake); err != nil {
		t.Fatalf("register: %v", err)
	}

	st := state.NewAppState()
	st.SetPendingWorkflowInit(true)

	engine := NewEngine(nil, reg, st, nil)
	in, _ := json.Marshal(map[string]any{"command": "echo x"})
	result, isErr := engine.invokeTool(
		context.Background(),
		nil,
		llm.ToolUseStart{ID: "t1", Name: "bash", Input: in},
		reg,
		st,
		Deps{},
		false,
		0,
	)
	if !isErr {
		t.Fatalf("expected pending-init block, got success: %s", result)
	}
	if !strings.Contains(result, "task_plan_init") {
		t.Fatalf("expected task_plan_init guidance, got: %s", result)
	}
	if fake.lastInput != nil {
		t.Fatal("tool should not have been executed")
	}
}

func TestInvokeToolBlocksWrongPhase(t *testing.T) {
	reg := tool.NewRegistry()
	fake := &fakeShellTool{
		name:       "bash",
		permission: types.BehaviorAllow,
	}
	if err := reg.Register(fake); err != nil {
		t.Fatalf("register: %v", err)
	}

	st := state.NewAppState()
	st.SetWorkflowCard(&state.WorkflowCard{
		Title:    "x",
		Category: state.WorkflowCategoryMigration,
		Phase:    state.WorkflowPhaseResearch,
	})

	engine := NewEngine(nil, reg, st, nil)
	in, _ := json.Marshal(map[string]any{"command": "ls"})
	result, isErr := engine.invokeTool(
		context.Background(),
		nil,
		llm.ToolUseStart{ID: "t2", Name: "bash", Input: in},
		reg,
		st,
		Deps{},
		false,
		0,
	)
	if !isErr {
		t.Fatalf("expected phase block, got: %s", result)
	}
	if !strings.Contains(result, "research") {
		t.Fatalf("expected phase mention, got: %s", result)
	}
}
