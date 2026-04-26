// Package query
// File: golden_test.go
// Description: query.Engine 怨⑤뱺 ?쒕굹由ъ삤 ?뚭? ?뚯뒪??
// Responsibility: 5媛??쒕굹由ъ삤瑜??ㅽ뻾?섏뿬 ?대깽???쒗?ㅼ? ?곕????ъ쑀瑜?寃利?

package query

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/tools"
)

// ??? mock LLM client ???????????????????????????????????????????????????????

type scenarioClient struct {
	turns []llm.Response
	idx   int
}

func (c *scenarioClient) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolDef, _ interface{}, opts ...llm.CallOption) (llm.Response, error) {
	return llm.Response{}, nil
}

func (c *scenarioClient) ChatStream(
	_ context.Context, _ []llm.Message, _ []llm.ToolDef, _ interface{},
	_ func(string), onToken func(string), opts ...llm.CallOption,
) (llm.Response, error) {
	if c.idx >= len(c.turns) {
		return llm.Response{}, nil
	}
	resp := c.turns[c.idx]
	c.idx++
	if onToken != nil && resp.Content != "" {
		onToken(resp.Content)
	}
	return resp, nil
}

// ??? mock blocking client ??????????????????????????????????????????????????

type blockingClient struct{ started *atomic.Bool }

func (c *blockingClient) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolDef, _ interface{}, opts ...llm.CallOption) (llm.Response, error) {
	return llm.Response{}, nil
}
func (c *blockingClient) ChatStream(ctx context.Context, _ []llm.Message, _ []llm.ToolDef, _ interface{}, _ func(string), _ func(string), opts ...llm.CallOption) (llm.Response, error) {
	c.started.Store(true)
	<-ctx.Done()
	return llm.Response{}, ctx.Err()
}

// ??? mock read-only tool ???????????????????????????????????????????????????

type readOnlyStub struct{ name string }

func (t *readOnlyStub) Name() string                       { return t.name }
func (t *readOnlyStub) Description() string                { return t.name }
func (t *readOnlyStub) Parameters() map[string]interface{} { return nil }
func (t *readOnlyStub) IsReadOnly() bool                   { return true }
func (t *readOnlyStub) IsEnabled() bool                    { return true }
func (t *readOnlyStub) Execute(_ context.Context, _ map[string]any, _ executor.Executor) (tools.ToolOutcome, error) {
	return tools.ToolOutcome{Content: t.name + " result", Success: true}, nil
}

// ??? helpers ???????????????????????????????????????????????????????????????

func gtc(id, name string) llm.ToolCall {
	return llm.ToolCall{ID: id, Type: "function", Function: llm.FunctionCall{Name: name, Arguments: "{}"}}
}

func toolCallTurn(calls ...llm.ToolCall) llm.Response {
	return llm.Response{ToolCalls: calls}
}

func textTurn(text string) llm.Response {
	return llm.Response{Content: text}
}

func collectEvents(ch <-chan QueryEvent) []QueryEvent {
	var evs []QueryEvent
	for ev := range ch {
		evs = append(evs, ev)
	}
	return evs
}

func findTerminal(t *testing.T, evs []QueryEvent) EventTerminal {
	t.Helper()
	for _, ev := range evs {
		if term, ok := ev.(EventTerminal); ok {
			return term
		}
	}
	t.Fatal("no EventTerminal received")
	return EventTerminal{}
}

func countType[T QueryEvent](evs []QueryEvent) int {
	n := 0
	for _, ev := range evs {
		if _, ok := ev.(T); ok {
			n++
		}
	}
	return n
}

func countSiblingSkipped(evs []QueryEvent) int {
	n := 0
	for _, ev := range evs {
		if r, ok := ev.(EventToolResult); ok && r.SiblingSkipped {
			n++
		}
	}
	return n
}

// ??? golden scenarios ??????????????????????????????????????????????????????

// TestGolden_01_EchoOnce: ?꾧뎄 1???몄텧 ??completed
func TestGolden_01_EchoOnce(t *testing.T) {
	client := &scenarioClient{turns: []llm.Response{
		toolCallTurn(gtc("1", "echo")),
		textTurn("?꾨즺?덉뒿?덈떎."),
	}}
	runTool := func(_ context.Context, tc llm.ToolCall) (string, bool, string) { return "hello", false, "" }

	eng := New(nil)
	evs := collectEvents(eng.Run(context.Background(), Params{
		Client: client, MaxTurns: 10, RunTool: runTool,
		SystemMsg: llm.Message{Role: llm.RoleSystem, Content: "test"},
	}))

	term := findTerminal(t, evs)
	if term.Reason != TerminalCompleted {
		t.Errorf("want completed, got %q", term.Reason)
	}
	if n := countType[EventToolResult](evs); n != 1 {
		t.Errorf("want 1 EventToolResult, got %d", n)
	}
}

// TestGolden_02_MultiTurn: 2 turn 媛??꾧뎄 1媛???completed
func TestGolden_02_MultiTurn(t *testing.T) {
	client := &scenarioClient{turns: []llm.Response{
		toolCallTurn(gtc("1", "read1")),
		toolCallTurn(gtc("2", "read2")),
		textTurn("???뚯씪 ?쎌쓬."),
	}}
	runTool := func(_ context.Context, tc llm.ToolCall) (string, bool, string) {
		return tc.Function.Name + " output", false, ""
	}

	eng := New(nil)
	evs := collectEvents(eng.Run(context.Background(), Params{
		Client: client, MaxTurns: 10, RunTool: runTool,
		SystemMsg: llm.Message{Role: llm.RoleSystem, Content: "test"},
	}))

	term := findTerminal(t, evs)
	if term.Reason != TerminalCompleted {
		t.Errorf("want completed, got %q", term.Reason)
	}
	if n := countType[EventToolResult](evs); n != 2 {
		t.Errorf("want 2 EventToolResult, got %d", n)
	}
}

// TestGolden_03_ParallelReadSerialWrite: read 3媛?蹂묐젹 + write 1媛?吏곷젹
func TestGolden_03_ParallelReadSerialWrite(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&readOnlyStub{name: "r1"})
	reg.Register(&readOnlyStub{name: "r2"})
	reg.Register(&readOnlyStub{name: "r3"})

	client := &scenarioClient{turns: []llm.Response{
		toolCallTurn(gtc("1", "r1"), gtc("2", "r2"), gtc("3", "r3"), gtc("4", "w1")),
		textTurn("?꾨즺."),
	}}
	runTool := func(_ context.Context, tc llm.ToolCall) (string, bool, string) {
		return tc.Function.Name + " done", false, ""
	}

	eng := New(nil)
	evs := collectEvents(eng.Run(context.Background(), Params{
		Client: client, MaxTurns: 10, RunTool: runTool, Registry: reg,
		SystemMsg: llm.Message{Role: llm.RoleSystem, Content: "test"},
	}))

	term := findTerminal(t, evs)
	if term.Reason != TerminalCompleted {
		t.Errorf("want completed, got %q", term.Reason)
	}
	if n := countType[EventToolResult](evs); n != 4 {
		t.Errorf("want 4 EventToolResult, got %d", n)
	}
}

// TestGolden_04_MutationFailSiblingSkip: mutation ?ㅽ뙣 ??sibling skip
func TestGolden_04_MutationFailSiblingSkip(t *testing.T) {
	client := &scenarioClient{turns: []llm.Response{
		toolCallTurn(gtc("1", "write1"), gtc("2", "write2")),
		textTurn("?ㅽ뙣?덉뒿?덈떎."),
	}}
	runTool := func(_ context.Context, tc llm.ToolCall) (string, bool, string) {
		if tc.Function.Name == "write1" {
			return "write failed", true, ""
		}
		return "write2 ok", false, ""
	}

	eng := New(nil)
	evs := collectEvents(eng.Run(context.Background(), Params{
		Client: client, MaxTurns: 10, RunTool: runTool,
		SystemMsg: llm.Message{Role: llm.RoleSystem, Content: "test"},
	}))

	term := findTerminal(t, evs)
	if term.Reason != TerminalCompleted {
		t.Errorf("want completed, got %q", term.Reason)
	}
	if n := countSiblingSkipped(evs); n != 1 {
		t.Errorf("want 1 sibling-skipped, got %d", n)
	}
}

// TestGolden_05_CtxCancelInterrupted: ctx cancel ??terminal interrupted
func TestGolden_05_CtxCancelInterrupted(t *testing.T) {
	var started atomic.Bool
	client := &blockingClient{started: &started}

	ctx, cancel := context.WithCancel(context.Background())
	eng := New(nil)
	events := eng.Run(ctx, Params{
		Client:    client,
		MaxTurns:  10,
		SystemMsg: llm.Message{Role: llm.RoleSystem, Content: "test"},
	})

	go func() {
		for !started.Load() {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()

	evs := collectEvents(events)
	term := findTerminal(t, evs)
	if term.Reason != TerminalInterrupted {
		t.Errorf("want interrupted, got %q", term.Reason)
	}
}
