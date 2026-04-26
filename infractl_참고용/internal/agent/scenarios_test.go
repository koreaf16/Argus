// Package agent
// File: scenarios_test.go
// Description: 다양한 도구 호출 시나리오 테스트
// Responsibility: 병렬/순차 실행, 에러 처리, 타겟팅 로직 검증

package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/tools"
)

type mockTool struct {
	name       string
	isReadOnly bool
	executeFn  func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (tools.ToolOutcome, error)
}

func (m *mockTool) Name() string        { return m.name }
func (m *mockTool) Description() string { return m.name + " description" }
func (m *mockTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object"}
}
func (m *mockTool) IsReadOnly() bool { return m.isReadOnly }
func (m *mockTool) IsEnabled() bool  { return true }
func (m *mockTool) Execute(ctx context.Context, args map[string]interface{}, exec executor.Executor) (tools.ToolOutcome, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, args, exec)
	}
	return tools.ToolOutcome{Content: "success", Success: true}, nil
}

func TestScenarios_ParallelReadOnly(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&mockTool{name: "read_1", isReadOnly: true})
	reg.Register(&mockTool{name: "read_2", isReadOnly: true})

	a := &Agent{
		registry: reg,
		manager:  executor.NewManager(noopAgentExecutor{}),
		handler:  noopAgentEventHandler{},
	}

	results := a.executeToolCalls(context.Background(), []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "read_1", Arguments: "{}"}},
		{ID: "c2", Function: llm.FunctionCall{Name: "read_2", Arguments: "{}"}},
	})

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Content != "success" || results[1].Content != "success" {
		t.Errorf("unexpected results: %+v", results)
	}
}

func TestScenarios_SequentialMutation(t *testing.T) {
	reg := tools.NewRegistry()
	order := []string{}
	reg.Register(&mockTool{
		name:       "mut_1",
		isReadOnly: false,
		executeFn: func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (tools.ToolOutcome, error) {
			order = append(order, "mut_1")
			return tools.ToolOutcome{Content: "ok1", Success: true}, nil
		},
	})
	reg.Register(&mockTool{
		name:       "mut_2",
		isReadOnly: false,
		executeFn: func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (tools.ToolOutcome, error) {
			order = append(order, "mut_2")
			return tools.ToolOutcome{Content: "ok2", Success: true}, nil
		},
	})

	a := &Agent{
		registry: reg,
		manager:  executor.NewManager(noopAgentExecutor{}),
		handler:  noopAgentEventHandler{},
	}

	a.executeToolCalls(context.Background(), []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "mut_1", Arguments: "{}"}},
		{ID: "c2", Function: llm.FunctionCall{Name: "mut_2", Arguments: "{}"}},
	})

	if len(order) != 2 || order[0] != "mut_1" || order[1] != "mut_2" {
		t.Fatalf("expected sequential execution mut_1 -> mut_2, got %v", order)
	}
}

func TestScenarios_UnknownTool(t *testing.T) {
	reg := tools.NewRegistry()
	a := &Agent{
		registry: reg,
		manager:  executor.NewManager(noopAgentExecutor{}),
		handler:  noopAgentEventHandler{},
	}

	results := a.executeToolCalls(context.Background(), []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "no_such_tool", Arguments: "{}"}},
	})

	if !strings.Contains(results[0].Content, "Error: unknown tool") {
		t.Errorf("expected error message for unknown tool, got %q", results[0].Content)
	}
}

func TestScenarios_InvalidArguments(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&mockTool{name: "tool_1"})

	a := &Agent{
		registry: reg,
		manager:  executor.NewManager(noopAgentExecutor{}),
		handler:  noopAgentEventHandler{},
	}

	results := a.executeToolCalls(context.Background(), []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "tool_1", Arguments: "invalid-json"}},
	})

	if !strings.Contains(results[0].Content, "Error: failed to parse arguments") {
		t.Errorf("expected parse error, got %q", results[0].Content)
	}
}

type targetTrackingExecutor struct {
	noopAgentExecutor
	target string
}

func (e *targetTrackingExecutor) Target() string { return e.target }
func (e *targetTrackingExecutor) Host() string   { return e.target }

func TestScenarios_TargetExecutorSelection(t *testing.T) {
	reg := tools.NewRegistry()
	var capturedTarget string
	reg.Register(&mockTool{
		name: "tool_1",
		executeFn: func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (tools.ToolOutcome, error) {
			capturedTarget = exec.Target()
			return tools.ToolOutcome{Content: "ok", Success: true}, nil
		},
	})

	mgr := executor.NewManager(&targetTrackingExecutor{target: "default"})
	mgr.Register("server-A", &targetTrackingExecutor{target: "server-A"})
	mgr.Register("server-B", &targetTrackingExecutor{target: "server-B"})

	a := &Agent{
		registry: reg,
		manager:  mgr,
		handler:  noopAgentEventHandler{},
	}

	// 1. Explicit target in args
	argsA, _ := json.Marshal(map[string]interface{}{"target": "server-A"})
	a.executeToolCalls(context.Background(), []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "tool_1", Arguments: string(argsA)}},
	})
	if capturedTarget != "server-A" {
		t.Errorf("expected target server-A, got %q", capturedTarget)
	}

	// 2. Default target (localhost/default)
	a.executeToolCalls(context.Background(), []llm.ToolCall{
		{ID: "c2", Function: llm.FunctionCall{Name: "tool_1", Arguments: "{}"}},
	})
	if capturedTarget != "localhost" && capturedTarget != "default" {
		t.Errorf("expected default target, got %q", capturedTarget)
	}
}

type scriptableExecutor struct {
	noopAgentExecutor
	scripts map[string]executor.ExecResult
}

func (e *scriptableExecutor) Execute(ctx context.Context, cmd string) (executor.ExecResult, error) {
	if res, ok := e.scripts[cmd]; ok {
		return res, nil
	}
	return executor.ExecResult{ExitCode: 0, Stdout: "default output"}, nil
}

func TestScenarios_ShellExecSuccess(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&tools.ShellExecTool{})

	exec := &scriptableExecutor{
		scripts: map[string]executor.ExecResult{
			"whoami": {ExitCode: 0, Stdout: "root"},
		},
	}

	a := &Agent{
		registry: reg,
		manager:  executor.NewManager(exec),
		handler:  noopAgentEventHandler{},
	}

	args, _ := json.Marshal(map[string]interface{}{"command": "whoami"})
	results := a.executeToolCalls(context.Background(), []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "shell_exec", Arguments: string(args)}},
	})

	if !strings.Contains(results[0].Content, "root") {
		t.Errorf("expected 'root' in output, got %q", results[0].Content)
	}
}

func TestScenarios_ShellExecFailure(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&tools.ShellExecTool{})

	exec := &scriptableExecutor{
		scripts: map[string]executor.ExecResult{
			"ls /nonexistent": {ExitCode: 2, Stdout: "ls: cannot access '/nonexistent': No such file or directory"},
		},
	}

	a := &Agent{
		registry: reg,
		manager:  executor.NewManager(exec),
		handler:  noopAgentEventHandler{},
	}

	args, _ := json.Marshal(map[string]interface{}{"command": "ls /nonexistent"})
	results := a.executeToolCalls(context.Background(), []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "shell_exec", Arguments: string(args)}},
	})

	if !strings.Contains(results[0].Content, "ls: cannot access") {
		t.Errorf("expected error message in output, got %q", results[0].Content)
	}
}

type mockQuestionHandler struct {
	response tools.QuestionResponse
}

func (h *mockQuestionHandler) RequestQuestion(ctx context.Context, req tools.QuestionRequest) (tools.QuestionResponse, error) {
	return h.response, nil
}
func (h *mockQuestionHandler) RequestForm(ctx context.Context, req tools.FormRequest) (tools.FormResponse, error) {
	return tools.FormResponse{}, nil
}

func TestScenarios_AskUserQuestion(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&tools.AskUserQuestionTool{})

	handler := &mockQuestionHandler{
		response: tools.QuestionResponse{
			SelectedLabel: "Yes",
			SelectedIndex: 0,
		},
	}

	a := &Agent{
		registry:        reg,
		manager:         executor.NewManager(noopAgentExecutor{}),
		handler:         noopAgentEventHandler{},
		questionHandler: handler,
	}

	args, _ := json.Marshal(map[string]interface{}{
		"question": "Proceed?",
		"options": []map[string]string{
			{"label": "Yes", "description": "Go ahead"},
			{"label": "No", "description": "Stop"},
		},
	})

	results := a.executeToolCalls(context.Background(), []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "ask_user_question", Arguments: string(args)}},
	})

	if !strings.Contains(results[0].Content, "Yes") {
		t.Errorf("expected 'Yes' in result, got %q", results[0].Content)
	}
}
