package todowrite

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/koreaf16/argus/internal/state"
	tool "github.com/koreaf16/argus/internal/tools"
)

func runTodoWrite(t *testing.T, st *state.AppState, raw string) {
	t.Helper()
	wd, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	tl := NewTodoWriteTool()
	ev, err := tl.Call(tool.Context{Context: context.Background(), State: st}, json.RawMessage(raw))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	for e := range ev {
		if e.Kind == tool.ToolEventError {
			t.Fatalf("tool error: %v", e.Err)
		}
	}
}

// 워크플로우 카드가 plan 단계인 상태에서 LLM 이 phase 필드 없이
// execute todo 를 in_progress 로 표시했을 때, 자동으로 phase 가 execute 로 전환되어야 한다.
// 이 폴백이 없으면 phase 가 plan 에 묶여 후속 도구가 모두 차단되는 데드락이 발생한다 (회귀 가드).
func TestTodoWriteInfersPhaseWhenFieldOmitted(t *testing.T) {
	st := state.NewAppState()
	st.SetSessionID("infer-test")
	st.SetWorkflowCard(&state.WorkflowCard{
		Title:    "Postgres install",
		Category: state.WorkflowCategoryInstall,
		Phase:    state.WorkflowPhasePlan,
	})

	raw := `{"todos":[
		{"content":"Discover environment","status":"completed","activeForm":"Discovering environment"},
		{"content":"Draft step-by-step plan","status":"completed","activeForm":"Drafting plan"},
		{"content":"Execute approved plan","status":"in_progress","activeForm":"Executing plan"},
		{"content":"Verify outcome","status":"pending","activeForm":"Verifying outcome"}
	]}`
	runTodoWrite(t, st, raw)

	if got := st.WorkflowCard().Phase; got != state.WorkflowPhaseExecute {
		t.Fatalf("expected phase to advance to execute via inference, got %q", got)
	}
}

// 명시적인 phase 필드가 있으면 그것이 우선해야 한다 (기존 동작 보호).
func TestTodoWriteExplicitPhaseWins(t *testing.T) {
	st := state.NewAppState()
	st.SetSessionID("explicit-test")
	st.SetWorkflowCard(&state.WorkflowCard{
		Title:    "x",
		Category: state.WorkflowCategoryInstall,
		Phase:    state.WorkflowPhasePlan,
	})

	raw := `{"phase":"verify","todos":[
		{"content":"Execute approved plan","status":"in_progress","activeForm":"Executing plan"}
	]}`
	runTodoWrite(t, st, raw)

	if got := st.WorkflowCard().Phase; got != state.WorkflowPhaseVerify {
		t.Fatalf("explicit phase should win, got %q", got)
	}
}

// 워크플로우 카드가 없으면 phase 추론을 적용하지 않아야 한다.
func TestTodoWriteSkipsInferenceWithoutCard(t *testing.T) {
	st := state.NewAppState()
	st.SetSessionID("nocard-test")

	raw := `{"todos":[
		{"content":"Execute approved plan","status":"in_progress","activeForm":"Executing plan"}
	]}`
	runTodoWrite(t, st, raw)

	if st.WorkflowCard() != nil {
		t.Fatal("inference must not materialize a workflow card")
	}
}

func TestTodoWriteRejectsNoopUpdate(t *testing.T) {
	st := state.NewAppState()
	st.SetSessionID("noop-test")

	raw := `{"todos":[
		{"content":"Draft step-by-step plan","status":"in_progress","activeForm":"Drafting plan"}
	]}`
	runTodoWrite(t, st, raw)

	tl := NewTodoWriteTool()
	ev, err := tl.Call(tool.Context{Context: context.Background(), State: st}, json.RawMessage(raw))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var gotErr error
	for e := range ev {
		if e.Kind == tool.ToolEventError {
			gotErr = e.Err
		}
	}
	if gotErr == nil || !strings.Contains(gotErr.Error(), "made no changes") {
		t.Fatalf("expected no-op error, got %v", gotErr)
	}
}
