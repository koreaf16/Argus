// Package query
// File: tool_invoker_test.go
// Description: ToolInvoker hook ?듯빀 ?ㅽ뻾 ?섑띁 ?⑥쐞 ?뚯뒪??

package query

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/yourorg/infractl/internal/agent/todo"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/hooks"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/tools"
)

func makeTC(name, args string) llm.ToolCall {
	return llm.ToolCall{
		ID:       "tc-1",
		Type:     "function",
		Function: llm.FunctionCall{Name: name, Arguments: args},
	}
}

type invokerStubTool struct {
	name     string
	readOnly bool
}

func (t invokerStubTool) Name() string                       { return t.name }
func (t invokerStubTool) Description() string                { return "stub tool" }
func (t invokerStubTool) Parameters() map[string]interface{} { return map[string]interface{}{} }
func (t invokerStubTool) IsReadOnly() bool                   { return t.readOnly }
func (t invokerStubTool) IsEnabled() bool                    { return true }
func (t invokerStubTool) Execute(context.Context, map[string]interface{}, executor.Executor) (tools.ToolOutcome, error) {
	return tools.ToolOutcome{Content: "stub", Success: true}, nil
}

// TestToolInvoker_NilHookRunner: hook runner ?놁쑝硫?base 瑜?吏곸젒 ?몄텧?쒕떎.
func TestToolInvoker_NilHookRunner(t *testing.T) {
	called := false
	base := func(_ context.Context, tc llm.ToolCall) (string, bool, string) {
		called = true
		return "ok", false, ""
	}
	ti := NewToolInvoker(nil, base)
	out, isErr, _ := ti.Invoke(context.Background(), makeTC("read", `{"path":"/tmp"}`))
	if !called {
		t.Error("base should be called when hookRunner is nil")
	}
	if isErr {
		t.Error("should not be error")
	}
	if out != "ok" {
		t.Errorf("unexpected output: %q", out)
	}
}

// TestToolInvoker_WithApprovedHook: Phase B hookRunner ????긽 approved=true.
// base 媛 ?몄텧?섍퀬 寃곌낵媛 洹몃?濡?諛섑솚?섏뼱???쒕떎.
func TestToolInvoker_WithApprovedHook(t *testing.T) {
	runner := hooks.NewRunner(nil)
	called := false
	base := func(_ context.Context, tc llm.ToolCall) (string, bool, string) {
		called = true
		return "result", false, ""
	}
	ti := NewToolInvoker(runner, base)
	out, isErr, _ := ti.Invoke(context.Background(), makeTC("bash", `{"command":"ls"}`))
	if !called {
		t.Error("base should be called when hook approves")
	}
	if isErr {
		t.Error("should not be error")
	}
	if out != "result" {
		t.Errorf("want %q, got %q", "result", out)
	}
}

// TestToolInvoker_BaseError: base 媛 isError=true 瑜?諛섑솚?섎㈃ 洹몃?濡??꾪뙆?쒕떎.
func TestToolInvoker_BaseError(t *testing.T) {
	runner := hooks.NewRunner(nil)
	base := func(_ context.Context, _ llm.ToolCall) (string, bool, string) {
		return "tool failed", true, ""
	}
	ti := NewToolInvoker(runner, base)
	out, isErr, _ := ti.Invoke(context.Background(), makeTC("bash", `{}`))
	if !isErr {
		t.Error("isError should propagate from base")
	}
	if out != "tool failed" {
		t.Errorf("unexpected output: %q", out)
	}
}

// TestToolInvoker_AsToolRunner: AsToolRunner 媛 諛섑솚?섎뒗 ?⑥닔??Invoke ? ?숈씪?섎떎.
func TestToolInvoker_AsToolRunner(t *testing.T) {
	base := func(_ context.Context, _ llm.ToolCall) (string, bool, string) { return "via runner", false, "" }
	ti := NewToolInvoker(nil, base)
	runner := ti.AsToolRunner()
	out, isErr, _ := runner(context.Background(), makeTC("read", `{}`))
	if isErr || out != "via runner" {
		t.Errorf("AsToolRunner: got %q isErr=%v", out, isErr)
	}
}

// TestToolInvoker_TodoEnforcerUsesRegistryReadOnly: registry 湲곕컲 read-only ?꾧뎄??todo 鍮꾩뼱 ?덉뼱???덉슜?쒕떎.
func TestToolInvoker_TodoEnforcerUsesRegistryReadOnly(t *testing.T) {
	reg := tools.NewRegistry()
	if err := reg.Register(invokerStubTool{name: "clarify", readOnly: true}); err != nil {
		t.Fatalf("register stub tool: %v", err)
	}
	called := false
	ti := NewToolInvoker(nil, func(_ context.Context, _ llm.ToolCall) (string, bool, string) {
		called = true
		return "allowed", false, ""
	})
	ti.SetRegistry(reg)
	ti.SetTodoEnforcer(todo.NewEnforcer(todo.NewStore()))

	out, isErr, _ := ti.Invoke(context.Background(), makeTC("clarify", `{"question":"need details"}`))
	if isErr {
		t.Fatalf("read-only tool should be allowed, got error output: %q", out)
	}
	if !called {
		t.Fatal("read-only tool should reach the base runner")
	}
	if out != "allowed" {
		t.Fatalf("unexpected output: %q", out)
	}
}

// TestToolInvoker_TodoEnforcerBlocksRegistryMutation: mutation ?꾧뎄??todo 鍮꾩뼱 ?덉쑝硫?怨꾩냽 李⑤떒?쒕떎.
func TestToolInvoker_TodoEnforcerBlocksRegistryMutation(t *testing.T) {
	reg := tools.NewRegistry()
	if err := reg.Register(invokerStubTool{name: "mutate", readOnly: false}); err != nil {
		t.Fatalf("register stub tool: %v", err)
	}
	called := false
	ti := NewToolInvoker(nil, func(_ context.Context, _ llm.ToolCall) (string, bool, string) {
		called = true
		return "should-not-run", false, ""
	})
	ti.SetRegistry(reg)
	ti.SetTodoEnforcer(todo.NewEnforcer(todo.NewStore()))

	out, isErr, _ := ti.Invoke(context.Background(), makeTC("mutate", `{}`))
	if !isErr {
		t.Fatal("mutation tool should be blocked when todo is empty")
	}
	if called {
		t.Fatal("blocked tool must not reach base runner")
	}
	if out == "" {
		t.Fatal("blocked tool should return a reason")
	}
}

// TestParseArgsForHook_Valid: ?щ컮瑜?JSON ? map ?쇰줈 ?뚯떛?쒕떎.
func TestParseArgsForHook_Valid(t *testing.T) {
	args := `{"key":"value","num":42}`
	m := parseArgsForHook(args)
	if m["key"] != "value" {
		t.Errorf("want key=value, got %v", m["key"])
	}
	if m["num"] != json.Number("42") && m["num"] != float64(42) {
		// json.Unmarshal ? number ??float64
		if m["num"] != float64(42) {
			t.Errorf("want num=42, got %v", m["num"])
		}
	}
}

// TestParseArgsForHook_Invalid: ?뚯떛 ?ㅽ뙣 ??鍮?map ??諛섑솚?쒕떎.
func TestParseArgsForHook_Invalid(t *testing.T) {
	m := parseArgsForHook("not json")
	if len(m) != 0 {
		t.Errorf("want empty map, got %v", m)
	}
}

// TestParseArgsForHook_Empty: 鍮?臾몄옄?대룄 ?먮윭 ?놁씠 鍮?map ??諛섑솚?쒕떎.
func TestParseArgsForHook_Empty(t *testing.T) {
	m := parseArgsForHook("")
	if len(m) != 0 {
		t.Errorf("want empty map for empty input, got %v", m)
	}
}
