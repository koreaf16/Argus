package query

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/state"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
)

type fakeShellTool struct {
	name       string
	readOnly   bool
	permission types.PermissionBehavior
	lastInput  map[string]any
}

func (f *fakeShellTool) Name() string { return f.name }
func (f *fakeShellTool) Description(tool.Context) string {
	return "fake shell tool"
}
func (f *fakeShellTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{"type": "object"}
}
func (f *fakeShellTool) IsReadOnly() bool { return f.readOnly }
func (f *fakeShellTool) MaxResultSizeChars() int {
	return 0
}
func (f *fakeShellTool) CheckPermission(tool.Context, json.RawMessage) (tool.PermissionResult, error) {
	return tool.PermissionResult{Behavior: f.permission}, nil
}
func (f *fakeShellTool) Call(_ tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	_ = json.Unmarshal(input, &f.lastInput)
	ch := make(chan tool.ToolEvent, 2)
	ch <- tool.NewOutputEvent("ok")
	ch <- tool.NewDoneEvent()
	close(ch)
	return ch, nil
}

func TestParsePlanExecutionReady(t *testing.T) {
	steps := parsePlanExecutionReady("exit_plan_mode", `{"allowed_prompts":[{"tool":"bash","prompt":"echo 1","server":"oracle-server","role":"source_mysql","channel":"source","as_user":"oracle","privilege_method":"sudo"},{"tool":"powershell","prompt":"Get-Date"},{"tool":"python","prompt":"print(1)"}]}`, false)
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	if steps[0].Tool != "bash" || steps[0].Prompt != "echo 1" {
		t.Fatalf("unexpected first step: %+v", steps[0])
	}
	if steps[0].Server != "oracle-server" {
		t.Fatalf("unexpected first step server: %+v", steps[0])
	}
	if steps[0].Role != "source_mysql" || steps[0].Channel != "source" || steps[0].AsUser != "oracle" || steps[0].PrivilegeMethod != "sudo" {
		t.Fatalf("unexpected first step execution context: %+v", steps[0])
	}
	if steps[1].Tool != "powershell" || steps[1].Prompt != "Get-Date" {
		t.Fatalf("unexpected second step: %+v", steps[1])
	}
}

func TestParsePlanExecutionReadyLegacyName(t *testing.T) {
	steps := parsePlanExecutionReady("ExitPlanMode", `{"allowed_prompts":[{"tool":"bash","prompt":"echo legacy"}]}`, false)
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}
	if steps[0].Tool != "bash" || steps[0].Prompt != "echo legacy" {
		t.Fatalf("unexpected step: %+v", steps[0])
	}
}

func TestExecutePlannedStepAutoApprovesAsk(t *testing.T) {
	reg := tool.NewRegistry()
	fake := &fakeShellTool{
		name:       "bash",
		permission: types.BehaviorAsk,
	}
	if err := reg.Register(fake); err != nil {
		t.Fatalf("register fake tool: %v", err)
	}

	engine := NewEngine(nil, reg, state.NewAppState(), nil)
	engine.SetDeps(context.Background(), Deps{WorkingDir: "C:\\repo"})
	engine.SetApprovalGate(ApprovalGate{
		Prompt: func(context.Context, string, json.RawMessage) (bool, error) {
			t.Fatalf("approval gate should not be called for auto-approved planned step")
			return false, nil
		},
	})

	out, err := engine.ExecutePlannedStep(context.Background(), PlannedStep{
		Tool:    "bash",
		Prompt:  "echo hello",
		Server:  "oracle-server",
		Role:    "source_mysql",
		Channel: "source",
	})
	if err != nil {
		t.Fatalf("execute planned step: %v", err)
	}
	if out != "ok" {
		t.Fatalf("unexpected output: %q", out)
	}
	if fake.lastInput["command"] != "echo hello" {
		t.Fatalf("unexpected command input: %+v", fake.lastInput)
	}
	if fake.lastInput["workdir"] != "C:\\repo" {
		t.Fatalf("unexpected workdir input: %+v", fake.lastInput)
	}
	if fake.lastInput["server"] != "oracle-server" {
		t.Fatalf("unexpected server input: %+v", fake.lastInput)
	}
	if fake.lastInput["role"] != "source_mysql" || fake.lastInput["channel"] != "source" {
		t.Fatalf("unexpected role/channel input: %+v", fake.lastInput)
	}
}

func TestExecutePlannedStepRespectsDeny(t *testing.T) {
	reg := tool.NewRegistry()
	fake := &fakeShellTool{
		name:       "powershell",
		permission: types.BehaviorDeny,
	}
	if err := reg.Register(fake); err != nil {
		t.Fatalf("register fake tool: %v", err)
	}

	engine := NewEngine(nil, reg, state.NewAppState(), nil)
	_, err := engine.ExecutePlannedStep(context.Background(), PlannedStep{
		Tool:   "powershell",
		Prompt: "Get-Process",
	})
	if err == nil {
		t.Fatal("expected deny error")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInvokeToolPassthroughRequiresApprovalForWritableTool(t *testing.T) {
	reg := tool.NewRegistry()
	fake := &fakeShellTool{
		name:       "bash",
		permission: types.BehaviorPassthrough,
	}
	if err := reg.Register(fake); err != nil {
		t.Fatalf("register fake tool: %v", err)
	}

	engine := NewEngine(nil, reg, state.NewAppState(), nil)
	input, err := json.Marshal(map[string]any{"command": "echo hello"})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	result, isErr := engine.invokeTool(
		context.Background(),
		nil,
		llm.ToolUseStart{ID: "test-1", Name: "bash", Input: input},
		reg,
		state.NewAppState(),
		Deps{},
		false,
		0,
	)
	if !isErr {
		t.Fatalf("expected permission error, got success result: %q", result)
	}
	if !strings.Contains(result, "permission denied") {
		t.Fatalf("unexpected error result: %q", result)
	}
	if fake.lastInput != nil {
		t.Fatalf("tool should not have been executed on passthrough without approval")
	}
}

func TestInvokeToolPassthroughAllowsReadOnlyTool(t *testing.T) {
	reg := tool.NewRegistry()
	fake := &fakeShellTool{
		name:       "web_search",
		readOnly:   true,
		permission: types.BehaviorPassthrough,
	}
	if err := reg.Register(fake); err != nil {
		t.Fatalf("register fake tool: %v", err)
	}

	engine := NewEngine(nil, reg, state.NewAppState(), nil)
	input, err := json.Marshal(map[string]any{"query": "argus"})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	result, isErr := engine.invokeTool(
		context.Background(),
		nil,
		llm.ToolUseStart{ID: "test-2", Name: "web_search", Input: input},
		reg,
		state.NewAppState(),
		Deps{},
		false,
		0,
	)
	if isErr {
		t.Fatalf("expected read-only passthrough to run, got error: %q", result)
	}
	if result != "ok" {
		t.Fatalf("unexpected output: %q", result)
	}
}

func TestInvokeToolAskPromptAllowExecutes(t *testing.T) {
	reg := tool.NewRegistry()
	fake := &fakeShellTool{
		name:       "bash",
		permission: types.BehaviorAsk,
	}
	if err := reg.Register(fake); err != nil {
		t.Fatalf("register fake tool: %v", err)
	}

	engine := NewEngine(nil, reg, state.NewAppState(), nil)
	input, err := json.Marshal(map[string]any{"command": "echo ok"})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	called := false
	result, isErr := engine.invokeTool(
		context.Background(),
		nil,
		llm.ToolUseStart{ID: "test-ask-allow", Name: "bash", Input: input},
		reg,
		state.NewAppState(),
		Deps{
			ApproveTool: ApprovalGate{
				Prompt: func(context.Context, string, json.RawMessage) (bool, error) {
					called = true
					return true, nil
				},
			},
		},
		false,
		0,
	)
	if isErr {
		t.Fatalf("expected success, got error: %q", result)
	}
	if result != "ok" {
		t.Fatalf("unexpected output: %q", result)
	}
	if !called {
		t.Fatalf("expected approval prompt to be called")
	}
}

func TestExecutePlannedStepBypassesWorkflowPhaseGate(t *testing.T) {
	reg := tool.NewRegistry()
	fake := &fakeShellTool{
		name:       "bash",
		permission: types.BehaviorAllow,
	}
	if err := reg.Register(fake); err != nil {
		t.Fatalf("register fake tool: %v", err)
	}

	st := state.NewAppState()
	st.SetWorkflowCard(&state.WorkflowCard{
		Title:    "install",
		Category: state.WorkflowCategoryInstall,
		Phase:    state.WorkflowPhasePlan,
	})

	engine := NewEngine(nil, reg, st, nil)
	out, err := engine.ExecutePlannedStep(context.Background(), PlannedStep{
		Tool:   "bash",
		Prompt: "echo approved",
	})
	if err != nil {
		t.Fatalf("planned step should bypass workflow phase gate after explicit approval: %v", err)
	}
	if out != "ok" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestRepeatedIdenticalToolCallsStopsTurn(t *testing.T) {
	stopToolUse := llm.StopReasonToolUse
	raw := mustRawJSON(t, map[string]any{"value": "same"})
	llmScript := &scriptedLLM{
		sequences: [][]llm.Event{
			{{Kind: llm.EventToolUseStart, ToolUse: &llm.ToolUseStart{ID: "t1", Name: "repeat_tool", Input: raw}}, {Kind: llm.EventStop, Stop: &stopToolUse}},
			{{Kind: llm.EventToolUseStart, ToolUse: &llm.ToolUseStart{ID: "t2", Name: "repeat_tool", Input: raw}}, {Kind: llm.EventStop, Stop: &stopToolUse}},
			{{Kind: llm.EventToolUseStart, ToolUse: &llm.ToolUseStart{ID: "t3", Name: "repeat_tool", Input: raw}}, {Kind: llm.EventStop, Stop: &stopToolUse}},
		},
	}

	reg := tool.NewRegistry()
	if err := reg.Register(&scriptedTool{
		name:       "repeat_tool",
		permission: types.BehaviorAllow,
		output:     "ok",
	}); err != nil {
		t.Fatalf("register repeat tool: %v", err)
	}

	engine := NewEngine(llmScript, reg, state.NewAppState(), nil)
	cfg := DefaultConfig()
	cfg.MaxToolIterations = 10
	cfg.PersistenceEnabled = false
	engine.SetConfig(cfg)

	stream, err := engine.SubmitMessage(context.Background(), "loop")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	var gotErr error
	toolUses := 0
	for evt := range stream {
		if evt.Kind == UIEventToolUse {
			toolUses++
		}
		if evt.Kind == UIEventError {
			gotErr = evt.Err
		}
	}
	if gotErr == nil || !strings.Contains(gotErr.Error(), "repeated identical tool calls") {
		t.Fatalf("expected repeated tool-call error, got %v", gotErr)
	}
	if toolUses != repeatedToolCallLimit {
		t.Fatalf("expected %d tool uses before stop, got %d", repeatedToolCallLimit, toolUses)
	}
}
