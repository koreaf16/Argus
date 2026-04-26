package query

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/tools"
)

func TestPhaseB_E2EReadOnlyFiveParallelAllResultsArrive(t *testing.T) {
	t.Setenv("INFRACTL_MAX_TOOL_CONCURRENCY", "5")

	reg := tools.NewRegistry()
	for i := 1; i <= 5; i++ {
		if err := reg.Register(&readOnlyStub{name: fmt.Sprintf("r%d", i)}); err != nil {
			t.Fatalf("register read tool: %v", err)
		}
	}

	client := &scenarioClient{turns: []llm.Response{
		toolCallTurn(
			gtc("1", "r1"),
			gtc("2", "r2"),
			gtc("3", "r3"),
			gtc("4", "r4"),
			gtc("5", "r5"),
		),
		textTurn("parallel complete"),
	}}

	var active atomic.Int32
	var maxActive atomic.Int32
	runTool := func(_ context.Context, tc llm.ToolCall) (string, bool, string) {
		now := active.Add(1)
		for {
			prev := maxActive.Load()
			if now <= prev || maxActive.CompareAndSwap(prev, now) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		active.Add(-1)
		return tc.Function.Name + " output", false, ""
	}

	evs := collectEvents(New(nil).Run(context.Background(), Params{
		Client:    client,
		MaxTurns:  10,
		RunTool:   runTool,
		Registry:  reg,
		SystemMsg: llm.Message{Role: llm.RoleSystem, Content: "test"},
	}))

	if term := findTerminal(t, evs); term.Reason != TerminalCompleted {
		t.Fatalf("terminal = %q, want completed", term.Reason)
	}
	if got := countType[EventToolResult](evs); got != 5 {
		t.Fatalf("tool results = %d, want 5", got)
	}
	if got := maxActive.Load(); got < 2 {
		t.Fatalf("max concurrent tools = %d, want at least 2", got)
	}
}

func TestPhaseB_E2ELongSession100TurnsStable(t *testing.T) {
	turns := make([]llm.Response, 0, 101)
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("call-%03d", i)
		tc := llm.ToolCall{ID: id, Type: "function", Function: llm.FunctionCall{
			Name:      "echo",
			Arguments: fmt.Sprintf(`{"seq":%d}`, i),
		}}
		turns = append(turns, toolCallTurn(tc))
	}
	turns = append(turns, textTurn("long session complete"))

	client := &scenarioClient{turns: turns}
	runTool := func(_ context.Context, tc llm.ToolCall) (string, bool, string) {
		return tc.ID + " ok", false, ""
	}

	evs := collectEvents(New(nil).Run(context.Background(), Params{
		Client:    client,
		MaxTurns:  150,
		RunTool:   runTool,
		SystemMsg: llm.Message{Role: llm.RoleSystem, Content: "test"},
	}))

	if term := findTerminal(t, evs); term.Reason != TerminalCompleted {
		t.Fatalf("terminal = %q, want completed", term.Reason)
	}
	if got := countType[EventToolResult](evs); got != 100 {
		t.Fatalf("tool results = %d, want 100", got)
	}
}
