package agent

import (
	"context"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/tools"
)

type noopAgentEventHandler struct{}

func (noopAgentEventHandler) OnThinking(string, string) {}
func (noopAgentEventHandler) OnThinkingToken(string)    {}
func (noopAgentEventHandler) OnToken(string)            {}
func (noopAgentEventHandler) OnToolStart(string, string, string, map[string]any) {
}
func (noopAgentEventHandler) OnToolOutput(string, string) {}
func (noopAgentEventHandler) OnToolEnd(string, string, string, time.Duration, bool, string) {
}
func (noopAgentEventHandler) OnToolCancelled(string, string, string, time.Duration) {}
func (noopAgentEventHandler) OnResponse(string)                                     {}
func (noopAgentEventHandler) OnError(error)                                         {}
func (noopAgentEventHandler) OnUsageUpdate(int, int, float64, int64)                {}
func (noopAgentEventHandler) OnJobComplete(int, string, bool)                       {}
func (noopAgentEventHandler) OnRAGContext(int)                                      {}

type captureOnStartHandler struct {
	noopAgentEventHandler
	args map[string]any
}

func (h *captureOnStartHandler) OnToolStart(_ string, _ string, _ string, args map[string]any) {
	h.args = maps.Clone(args)
}

type noopAgentExecutor struct{}

func (noopAgentExecutor) Execute(context.Context, string) (executor.ExecResult, error) {
	return executor.ExecResult{ExitCode: 0}, nil
}

func (noopAgentExecutor) Target() string { return "localhost" }
func (noopAgentExecutor) Host() string   { return "localhost" }

type countingAgentExecutor struct {
	noopAgentExecutor
	calls int
}

func (e *countingAgentExecutor) Execute(context.Context, string) (executor.ExecResult, error) {
	e.calls++
	return executor.ExecResult{ExitCode: 0}, nil
}

type countingMutationTool struct {
	name    string
	calls   *int
	success bool
}

type readonlyWebSearchTool struct{}

func (readonlyWebSearchTool) Name() string        { return "web_search" }
func (readonlyWebSearchTool) Description() string { return "test web search tool" }
func (readonlyWebSearchTool) Parameters() map[string]any {
	return map[string]any{"type": "object"}
}
func (readonlyWebSearchTool) IsReadOnly() bool { return true }
func (readonlyWebSearchTool) IsEnabled() bool  { return true }
func (readonlyWebSearchTool) Execute(context.Context, map[string]any, executor.Executor) (tools.ToolOutcome, error) {
	return tools.ToolOutcome{Content: "ok", Success: true}, nil
}

func (t countingMutationTool) Name() string        { return t.name }
func (t countingMutationTool) Description() string { return "test mutation tool" }
func (t countingMutationTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
	}
}
func (t countingMutationTool) IsReadOnly() bool { return false }
func (t countingMutationTool) IsEnabled() bool  { return true }
func (t countingMutationTool) Execute(context.Context, map[string]any, executor.Executor) (tools.ToolOutcome, error) {
	*t.calls = *t.calls + 1
	return tools.ToolOutcome{
		Content:  t.name + "-result",
		Success:  t.success,
		ExitCode: map[bool]int{true: 0, false: 1}[t.success],
	}, nil
}

func TestExecuteToolCallsStopsMutationChainAfterFailure(t *testing.T) {
	reg := tools.NewRegistry()
	failCalls := 0
	nextCalls := 0
	lastCalls := 0
	if err := reg.Register(countingMutationTool{name: "mut_fail", calls: &failCalls, success: false}); err != nil {
		t.Fatalf("register fail tool: %v", err)
	}
	if err := reg.Register(countingMutationTool{name: "mut_next", calls: &nextCalls, success: true}); err != nil {
		t.Fatalf("register next tool: %v", err)
	}
	if err := reg.Register(countingMutationTool{name: "mut_last", calls: &lastCalls, success: true}); err != nil {
		t.Fatalf("register last tool: %v", err)
	}

	a := &Agent{
		registry: reg,
		manager:  executor.NewManager(noopAgentExecutor{}),
		handler:  noopAgentEventHandler{},
	}

	results := a.executeToolCalls(context.Background(), []llm.ToolCall{
		{
			ID:   "call-1",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "mut_fail",
				Arguments: `{}`,
			},
		},
		{
			ID:   "call-2",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "mut_next",
				Arguments: `{}`,
			},
		},
		{
			ID:   "call-3",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "mut_last",
				Arguments: `{}`,
			},
		},
	})

	if failCalls != 1 {
		t.Fatalf("expected mut_fail to execute once, got %d", failCalls)
	}
	if nextCalls != 0 || lastCalls != 0 {
		t.Fatalf("expected later mutation tools to be skipped, got mut_next=%d mut_last=%d", nextCalls, lastCalls)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 tool results, got %d", len(results))
	}
	if !strings.Contains(results[1].Content, "Skipped:") || !strings.Contains(results[2].Content, "Skipped:") {
		t.Fatalf("expected skipped messages for remaining mutations, got: %#v", results)
	}
}

func TestExecuteSingleToolRewritesWebSearchQueryForActiveRockyServer(t *testing.T) {
	reg := tools.NewRegistry()
	if err := reg.Register(readonlyWebSearchTool{}); err != nil {
		t.Fatalf("register web search tool: %v", err)
	}

	handler := &captureOnStartHandler{}
	mgr := executor.NewManager(noopAgentExecutor{})
	mgr.Register("sandbox", noopAgentExecutor{})
	a := &Agent{
		registry: reg,
		manager:  mgr,
		handler:  handler,
		activeServer: &store.Server{
			Name: "sandbox",
			OS:   "Rocky Linux 9.7 (Blue Onyx)",
		},
	}

	msg, ok, _ := a.executeSingleTool(context.Background(), llm.ToolCall{
		ID:   "call-web-1",
		Type: "function",
		Function: llm.FunctionCall{
			Name:      "web_search",
			Arguments: `{"query":"Oracle 19c installation guide"}`,
		},
	})
	if !ok {
		t.Fatalf("expected tool success, got message=%+v", msg)
	}

	got, _ := handler.args["query"].(string)
	for _, want := range []string{"Rocky Linux 9", "RHEL 9 compatible"} {
		if !containsFold(got, want) {
			t.Fatalf("expected %q in rewritten query, got %q", want, got)
		}
	}
}

func TestExecuteSingleToolAcceptsEscapedDollarInJSONArgs(t *testing.T) {
	reg := tools.NewRegistry()
	if err := reg.Register(&tools.ShellExecTool{}); err != nil {
		t.Fatalf("register shell_exec: %v", err)
	}

	handler := &captureOnStartHandler{}
	a := &Agent{
		registry: reg,
		manager:  executor.NewManager(noopAgentExecutor{}),
		handler:  handler,
	}

	msg, ok, _ := a.executeSingleTool(context.Background(), llm.ToolCall{
		ID:   "shell-dollar-1",
		Type: "function",
		Function: llm.FunctionCall{
			Name:      "shell_exec",
			Arguments: `{"command":"echo 'SELECT name FROM v\\$pdbs;'","target":"localhost"}`,
		},
	})
	if !ok {
		t.Fatalf("expected shell_exec success, got %q", msg.Content)
	}

	got, _ := handler.args["command"].(string)
	if !strings.Contains(got, `v\$pdbs`) {
		t.Fatalf("expected escaped dollar to survive JSON parsing, got %q", got)
	}
}

func TestExecuteSingleToolVerifyCompleteClosesPendingShellTask(t *testing.T) {
	reg := tools.NewRegistry()
	if err := reg.Register(&tools.ShellExecTool{}); err != nil {
		t.Fatalf("register shell_exec: %v", err)
	}
	if err := reg.Register(&tools.VerifyCompleteTool{}); err != nil {
		t.Fatalf("register verify_complete: %v", err)
	}

	a := &Agent{
		registry: reg,
		manager:  executor.NewManager(noopAgentExecutor{}),
		handler:  noopAgentEventHandler{},
	}

	shellMsg, ok, _ := a.executeSingleTool(context.Background(), llm.ToolCall{
		ID:   "shell-1",
		Type: "function",
		Function: llm.FunctionCall{
			Name:      "shell_exec",
			Arguments: `{"command":"dnf install -y nginx","description":"nginx 설치"}`,
		},
	})
	if !ok {
		t.Fatalf("expected shell_exec success, got %q", shellMsg.Content)
	}

	pending := a.pendingVerificationTasks()
	if len(pending) != 1 {
		t.Fatalf("pending verification tasks = %d, want 1", len(pending))
	}
	if pending[0].ToolID != "shell-1" {
		t.Fatalf("pending tool id = %q, want shell-1", pending[0].ToolID)
	}

	verifyMsg, ok, _ := a.executeSingleTool(context.Background(), llm.ToolCall{
		ID:   "verify-1",
		Type: "function",
		Function: llm.FunctionCall{
			Name:      "verify_complete",
			Arguments: `{"tool_id":"shell-1","summary":"nginx package installed","verification_evidence":"rpm -q nginx returned installed"}`,
		},
	})
	if !ok {
		t.Fatalf("expected verify_complete success, got %q", verifyMsg.Content)
	}

	if len(a.pendingVerificationTasks()) != 0 {
		t.Fatalf("expected pending verification list to be empty after verify_complete")
	}

	meta, ok := a.taskProgress["shell-1"]
	if !ok {
		t.Fatal("expected shell task progress to be stored")
	}
	if meta.VerificationStatus != "verified" {
		t.Fatalf("verification status = %q, want verified", meta.VerificationStatus)
	}
	if meta.VerifiedByToolID != "verify-1" {
		t.Fatalf("verified by tool id = %q, want verify-1", meta.VerifiedByToolID)
	}
}

func TestPendingVerificationNudgeListsShellTasks(t *testing.T) {
	a := &Agent{
		taskProgress: map[string]tools.TaskProgressMetadata{
			"shell-1": {
				TaskSummary:          "Oracle preinstall 설치",
				TaskKey:              "oracle-preinstall",
				ExecutionStatus:      "succeeded",
				VerificationRequired: true,
				VerificationStatus:   "pending",
				VerificationHint:     "rpm -q oracle-database-preinstall-19c",
			},
		},
	}

	nudge, ok := a.pendingVerificationNudge()
	if !ok {
		t.Fatal("expected pending verification nudge")
	}
	for _, want := range []string{"tool_id=shell-1", "task_key=oracle-preinstall", "Oracle preinstall 설치"} {
		if !strings.Contains(nudge.Content, want) {
			t.Fatalf("nudge content = %q, missing %q", nudge.Content, want)
		}
	}
}
