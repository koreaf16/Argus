package taskplaninit

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/koreaf16/argus/internal/state"
	"github.com/koreaf16/argus/internal/todostore"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
)

func newCallContext(t *testing.T, st *state.AppState) tool.Context {
	t.Helper()
	return tool.Context{
		Context: context.Background(),
		State:   st,
	}
}

func runTool(t *testing.T, raw string, st *state.AppState) string {
	t.Helper()
	wd, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	tl := New()
	ev, err := tl.Call(newCallContext(t, st), json.RawMessage(raw))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var output string
	for e := range ev {
		switch e.Kind {
		case tool.ToolEventOutput:
			output = e.Output
		case tool.ToolEventError:
			t.Fatalf("tool error: %v", e.Err)
		}
	}
	return output
}

func TestCallRegistersSixPhases(t *testing.T) {
	st := state.NewAppState()
	st.SetSessionID("test-session")
	out := runTool(t, `{"title":"Postgres 14 to 17 migration","category":"migration","workspace_roles":{"source_mysql":"oracle-server","target_postgres":"sandbox-server"}}`, st)
	if out == "" {
		t.Fatal("expected output payload")
	}
	card := st.WorkflowCard()
	if card == nil {
		t.Fatal("expected workflow card to be set")
	}
	if card.Phase != state.WorkflowPhaseDiscover {
		t.Fatalf("expected phase=discover, got %s", card.Phase)
	}
	if card.Category != state.WorkflowCategoryMigration {
		t.Fatalf("expected migration category, got %s", card.Category)
	}
	if card.WorkspaceRoles["source_mysql"] != "oracle-server" {
		t.Fatalf("expected workspace role source_mysql=oracle-server, got %v", card.WorkspaceRoles)
	}
	todos, _ := todostore.Load(todostore.SessionID(st.SessionID()))
	if len(todos) != 6 {
		t.Fatalf("expected 6 todos, got %d", len(todos))
	}
	if todos[0].Status != types.TodoStatusInProgress {
		t.Fatalf("expected first todo in_progress, got %s", todos[0].Status)
	}
}

// 새 OS 프로파일 필드 (os_family, os_version, package_manager, architecture) 가
// task_plan_init 입력에서 정규화되어 WorkflowCard 까지 전달되는지 회귀 가드.
// 이 데이터 흐름이 깨지면 execute phase 에서 sudo/dnf 변환에 필요한 컨텍스트가 손실된다.
func TestCallPersistsOSProfileFields(t *testing.T) {
	st := state.NewAppState()
	st.SetSessionID("os-profile-test")
	out := runTool(t, `{
		"title":"Postgres install",
		"category":"install",
		"workspace_roles":{"target_postgres":"sandbox-server"},
		"workspace_role_profiles":{
			"target_postgres":{
				"channel":"target",
				"server":"sandbox-server",
				"as_user":"postgres",
				"privilege_method":"SUDO",
				"os_family":"RHEL",
				"os_version":"9.4",
				"package_manager":"DNF",
				"architecture":"x86_64"
			}
		}
	}`, st)
	if out == "" {
		t.Fatal("expected output payload")
	}
	card := st.WorkflowCard()
	if card == nil {
		t.Fatal("expected workflow card to be set")
	}
	profile, ok := card.WorkspaceRoleProfiles["target_postgres"]
	if !ok {
		t.Fatalf("expected target_postgres profile, got %+v", card.WorkspaceRoleProfiles)
	}
	if profile.OSFamily != "rhel" {
		t.Fatalf("os_family should be normalized to lowercase: %q", profile.OSFamily)
	}
	if profile.OSVersion != "9.4" {
		t.Fatalf("os_version mismatch: %q", profile.OSVersion)
	}
	if profile.PackageManager != "dnf" {
		t.Fatalf("package_manager should be normalized to lowercase: %q", profile.PackageManager)
	}
	if profile.Architecture != "x86_64" {
		t.Fatalf("architecture mismatch: %q", profile.Architecture)
	}
	if profile.PrivilegeMethod != "sudo" {
		t.Fatalf("privilege_method should be normalized: %q", profile.PrivilegeMethod)
	}
}

func TestCallSkipsResearchPhase(t *testing.T) {
	st := state.NewAppState()
	st.SetSessionID("test-skip-research")
	runTool(t, `{"title":"Quick install","category":"install","needs_research":false}`, st)
	todos, _ := todostore.Load(todostore.SessionID(st.SessionID()))
	if len(todos) != 5 {
		t.Fatalf("expected 5 todos (research skipped), got %d", len(todos))
	}
	for _, td := range todos {
		if td.Content == phaseTodoContent(state.WorkflowPhaseResearch) {
			t.Fatalf("research todo should be skipped: %+v", td)
		}
	}
}

func TestCallSkipsInterviewPhase(t *testing.T) {
	st := state.NewAppState()
	st.SetSessionID("test-skip-interview")
	runTool(t, `{"title":"Self-contained tune","category":"tuning","needs_user_input":false}`, st)
	todos, _ := todostore.Load(todostore.SessionID(st.SessionID()))
	if len(todos) != 5 {
		t.Fatalf("expected 5 todos (interview skipped), got %d", len(todos))
	}
}

func TestCallShortcutForOtherCategoryWithBothSkipped(t *testing.T) {
	st := state.NewAppState()
	st.SetSessionID("test-shortcut")
	runTool(t, `{"title":"trivial","category":"other","needs_research":false,"needs_user_input":false}`, st)
	card := st.WorkflowCard()
	if card == nil || card.Phase != state.WorkflowPhaseExecute {
		t.Fatalf("expected shortcut to execute phase, got %+v", card)
	}
}

func TestCallClearsPendingFlag(t *testing.T) {
	st := state.NewAppState()
	st.SetSessionID("test-pending")
	st.SetPendingWorkflowInit(true)
	runTool(t, `{"title":"x","category":"install"}`, st)
	if st.PendingWorkflowInit() {
		t.Fatal("expected pending flag to be cleared after init")
	}
}

func TestCallRejectsEmptyTitle(t *testing.T) {
	st := state.NewAppState()
	tl := New()
	ev, err := tl.Call(newCallContext(t, st), json.RawMessage(`{"category":"install"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	gotErr := false
	for e := range ev {
		if e.Kind == tool.ToolEventError {
			gotErr = true
		}
	}
	if !gotErr {
		t.Fatal("expected error event for missing title")
	}
}

func TestPhaseAllowedToolsCoverage(t *testing.T) {
	for _, p := range []string{
		state.WorkflowPhaseDiscover,
		state.WorkflowPhaseResearch,
		state.WorkflowPhaseInterview,
		state.WorkflowPhasePlan,
		state.WorkflowPhaseVerify,
	} {
		if got := PhaseAllowedTools(p); len(got) == 0 {
			t.Fatalf("phase %s should have allowed tools", p)
		}
	}
	if got := PhaseAllowedTools(state.WorkflowPhaseExecute); got != nil {
		t.Fatalf("execute phase should be unrestricted (nil), got %v", got)
	}
	if got := PhaseAllowedTools(state.WorkflowPhaseDone); got != nil {
		t.Fatalf("done phase should be unrestricted (nil), got %v", got)
	}
}

func TestInferPhaseFromTodos(t *testing.T) {
	cases := []struct {
		name  string
		todos []types.TodoItem
		want  string
	}{
		{
			name: "canonical activeForm — Executing",
			todos: []types.TodoItem{
				{Content: "Execute approved plan steps", ActiveForm: "Executing plan", Status: types.TodoStatusInProgress},
			},
			want: state.WorkflowPhaseExecute,
		},
		{
			name: "canonical content — Verify outcome",
			todos: []types.TodoItem{
				{Content: "Verify outcome and report", ActiveForm: "", Status: types.TodoStatusInProgress},
			},
			want: state.WorkflowPhaseVerify,
		},
		{
			name: "parenthesized phase tag — LLM Korean style",
			todos: []types.TodoItem{
				{Content: "PostgreSQL 설치 실행 (execute)", ActiveForm: "PostgreSQL 설치 중", Status: types.TodoStatusInProgress},
			},
			want: state.WorkflowPhaseExecute,
		},
		{
			name: "drafting -> plan",
			todos: []types.TodoItem{
				{Content: "Draft step-by-step plan", ActiveForm: "Drafting plan", Status: types.TodoStatusInProgress},
			},
			want: state.WorkflowPhasePlan,
		},
		{
			name: "ignores completed/pending todos",
			todos: []types.TodoItem{
				{Content: "Discover environment", ActiveForm: "Discovering environment", Status: types.TodoStatusCompleted},
				{Content: "Verify outcome", ActiveForm: "Verifying outcome", Status: types.TodoStatusPending},
				{Content: "Execute approved plan", ActiveForm: "Executing plan", Status: types.TodoStatusInProgress},
			},
			want: state.WorkflowPhaseExecute,
		},
		{
			name: "no in_progress todo",
			todos: []types.TodoItem{
				{Content: "Execute approved plan", ActiveForm: "Executing plan", Status: types.TodoStatusCompleted},
			},
			want: "",
		},
		{
			name: "no recognizable signal",
			todos: []types.TodoItem{
				{Content: "Run installation commands", ActiveForm: "Running stuff", Status: types.TodoStatusInProgress},
			},
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := InferPhaseFromTodos(c.todos); got != c.want {
				t.Fatalf("InferPhaseFromTodos = %q, want %q", got, c.want)
			}
		})
	}
}

func TestNormalizeCategoryFallback(t *testing.T) {
	cases := map[string]string{
		"install":     state.WorkflowCategoryInstall,
		"  Migration": state.WorkflowCategoryMigration,
		"unknown":     state.WorkflowCategoryOther,
		"":            state.WorkflowCategoryOther,
	}
	for in, want := range cases {
		if got := normalizeCategory(in); got != want {
			t.Fatalf("normalizeCategory(%q) = %q, want %q", in, got, want)
		}
	}
}
