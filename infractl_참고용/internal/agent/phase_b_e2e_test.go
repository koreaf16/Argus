package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/tools"
)

type phaseBLLMClient struct {
	streamCalls atomic.Int32
	streamIndex int
	streamTurns []llm.Response
}

func (c *phaseBLLMClient) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolDef, _ interface{}, opts ...llm.CallOption) (llm.Response, error) {
	return llm.Response{Content: `{"kind":"new_request","target":"none","reason":"no pending context"}`}, nil
}

func (c *phaseBLLMClient) ChatStream(
	_ context.Context,
	_ []llm.Message,
	_ []llm.ToolDef,
	_ interface{},
	_ func(string),
	onToken func(string),
	opts ...llm.CallOption,
) (llm.Response, error) {
	c.streamCalls.Add(1)
	if c.streamIndex >= len(c.streamTurns) {
		return llm.Response{}, nil
	}
	resp := c.streamTurns[c.streamIndex]
	c.streamIndex++
	if onToken != nil && resp.Content != "" {
		onToken(resp.Content)
	}
	return resp, nil
}

type phaseBHandler struct {
	noopAgentEventHandler
	response string
}

func (h *phaseBHandler) OnResponse(content string) {
	h.response = content
}

func TestPhaseB_E2EAgentRunUsesQueryEngineAndClassifiesOnce(t *testing.T) {
	reg := tools.NewRegistry()
	var executedCommand string
	if err := reg.Register(&mockTool{
		name:       "shell_exec",
		isReadOnly: false,
		executeFn: func(_ context.Context, args map[string]interface{}, _ executor.Executor) (tools.ToolOutcome, error) {
			executedCommand, _ = args["command"].(string)
			return tools.ToolOutcome{Content: "phase-b tool ok", Success: true}, nil
		},
	}); err != nil {
		t.Fatalf("register shell_exec: %v", err)
	}

	args, _ := json.Marshal(map[string]any{"command": "echo phase-b"})
	client := &phaseBLLMClient{streamTurns: []llm.Response{
		{ToolCalls: []llm.ToolCall{{
			ID:       "tool-1",
			Function: llm.FunctionCall{Name: "shell_exec", Arguments: string(args)},
		}}},
		{Content: "done through query engine"},
	}}
	handler := &phaseBHandler{}
	agent := New(client, reg, executor.NewManager(noopAgentExecutor{}), handler, nil)

	if err := agent.Run(context.Background(), "ls 실행해줘"); err != nil {
		t.Fatalf("Agent.Run() error = %v", err)
	}
	if got := client.streamCalls.Load(); got != 2 {
		t.Fatalf("ChatStream calls = %d, want 2", got)
	}
	if executedCommand != "echo phase-b" {
		t.Fatalf("executed command = %q, want echo phase-b", executedCommand)
	}
	if handler.response != "done through query engine" {
		t.Fatalf("final response = %q", handler.response)
	}
}

type phaseBPTYExecutor struct {
	ptyUsed atomic.Bool
	command string
}

func (e *phaseBPTYExecutor) Execute(_ context.Context, command string) (executor.ExecResult, error) {
	e.command = command
	return executor.ExecResult{ExitCode: 0, Stdout: "direct"}, nil
}

func (e *phaseBPTYExecutor) ExecuteStream(_ context.Context, command string, onLine func(string)) (executor.ExecSession, error) {
	e.command = command
	if onLine != nil {
		onLine("stream")
	}
	return phaseBSession{result: executor.ExecResult{ExitCode: 0, Stdout: "stream"}}, nil
}

func (e *phaseBPTYExecutor) ExecuteStreamPTY(_ context.Context, command string, onLine func(string)) (executor.ExecSession, error) {
	e.ptyUsed.Store(true)
	e.command = command
	if onLine != nil {
		onLine("pty")
	}
	return phaseBSession{result: executor.ExecResult{ExitCode: 0, Stdout: "pty"}}, nil
}

func (e *phaseBPTYExecutor) Target() string { return "localhost" }
func (e *phaseBPTYExecutor) Host() string   { return "localhost" }

type phaseBSession struct {
	result executor.ExecResult
}

func (s phaseBSession) InjectStdin(string) error { return nil }
func (s phaseBSession) SendEOF() error           { return nil }
func (s phaseBSession) Wait() (executor.ExecResult, error) {
	return s.result, nil
}

type phaseBIdleHandler struct{}

func (phaseBIdleHandler) RequestIdleInput(context.Context, IdleInputRequest) (IdleInputResponse, error) {
	return IdleInputResponse{}, nil
}

func TestPhaseB_E2EInteractiveShellExecStillUsesPTYStream(t *testing.T) {
	reg := tools.NewRegistry()
	if err := reg.Register(&tools.ShellExecTool{}); err != nil {
		t.Fatalf("register shell_exec: %v", err)
	}

	args, _ := json.Marshal(map[string]any{"command": "vim"})
	client := &phaseBLLMClient{streamTurns: []llm.Response{
		{ToolCalls: []llm.ToolCall{{
			ID:       "tool-pty",
			Function: llm.FunctionCall{Name: "shell_exec", Arguments: string(args)},
		}}},
		{Content: "interactive path preserved"},
	}}
	ptyExec := &phaseBPTYExecutor{}
	handler := &phaseBHandler{}
	agent := New(client, reg, executor.NewManager(ptyExec), handler, nil)
	agent.SetIdleInputHandler(phaseBIdleHandler{})

	if err := agent.Run(context.Background(), "vim 실행해줘"); err != nil {
		t.Fatalf("Agent.Run() error = %v", err)
	}
	if !ptyExec.ptyUsed.Load() {
		t.Fatal("expected shell_exec to use PTY streaming through the query engine path")
	}
	if !strings.Contains(ptyExec.command, "vim") {
		t.Fatalf("PTY command = %q, want vim", ptyExec.command)
	}
	if handler.response != "interactive path preserved" {
		t.Fatalf("final response = %q", handler.response)
	}
}
