package query

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/yourorg/infractl/internal/agent/compact"
	"github.com/yourorg/infractl/internal/llm"
)

type phaseCLLMClient struct {
	turns       []func(opts ...llm.CallOption) (llm.Response, error)
	currentTurn int
	lastOpts    []llm.CallOption
}

func (c *phaseCLLMClient) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolDef, _ interface{}, opts ...llm.CallOption) (llm.Response, error) {
	return llm.Response{}, nil
}

func (c *phaseCLLMClient) ChatStream(_ context.Context, _ []llm.Message, _ []llm.ToolDef, _ interface{}, _ func(string), _ func(string), opts ...llm.CallOption) (llm.Response, error) {
	c.lastOpts = opts
	if c.currentTurn >= len(c.turns) {
		return llm.Response{}, nil
	}
	f := c.turns[c.currentTurn]
	c.currentTurn++
	return f(opts...)
}

type mockReactive struct{ called bool }

func (m *mockReactive) TryCompact(context.Context, *compact.State) (compact.CompactionResult, error) {
	m.called = true
	return compact.CompactionResult{}, nil
}

type mockCollapse struct{}

func (m *mockCollapse) ApplyIfNeeded(*compact.State, compact.TokenState) {}
func (m *mockCollapse) Drain(*compact.State) bool                        { return false }
func (m *mockCollapse) HasStaged() bool                                  { return false }

func TestPhaseC_E2ERecovery_PTL(t *testing.T) {
	client := &phaseCLLMClient{
		turns: []func(opts ...llm.CallOption) (llm.Response, error){
			func(opts ...llm.CallOption) (llm.Response, error) {
				// return PTL error
				return llm.Response{}, errors.New("prompt_too_long")
			},
			func(opts ...llm.CallOption) (llm.Response, error) {
				// success on retry
				return textTurn("recovered"), nil
			},
		},
	}

	react := &mockReactive{}
	recovery := compact.NewRecovery(&mockCollapse{}, react, client)

	eng := New(nil)
	eng.SetRecovery(recovery)

	evs := collectEvents(eng.Run(context.Background(), Params{
		Client:    client,
		MaxTurns:  10,
		SystemMsg: llm.Message{Role: llm.RoleSystem, Content: "test"},
	}))

	term := findTerminal(t, evs)
	if term.Reason != TerminalCompleted {
		t.Fatalf("want completed, got %q (err: %v)", term.Reason, term.Err)
	}
	if !react.called {
		t.Fatal("expected reactive compaction to be called")
	}
}

func TestPhaseC_E2ERecovery_MaxTokens(t *testing.T) {
	var maxTokensSent *int
	client := &phaseCLLMClient{
		turns: []func(opts ...llm.CallOption) (llm.Response, error){
			func(opts ...llm.CallOption) (llm.Response, error) {
				// return max tokens error
				return llm.Response{}, errors.New("stop_reason: max_tokens")
			},
			func(opts ...llm.CallOption) (llm.Response, error) {
				callOpts := &llm.CallOptions{}
				for _, o := range opts {
					o(callOpts)
				}
				maxTokensSent = callOpts.MaxTokens
				return textTurn("recovered"), nil
			},
		},
	}

	recovery := compact.NewRecovery(&mockCollapse{}, &mockReactive{}, client)

	eng := New(nil)
	eng.SetRecovery(recovery)

	evs := collectEvents(eng.Run(context.Background(), Params{
		Client:    client,
		MaxTurns:  10,
		SystemMsg: llm.Message{Role: llm.RoleSystem, Content: "test"},
	}))

	term := findTerminal(t, evs)
	if term.Reason != TerminalCompleted {
		t.Fatalf("want completed, got %q", term.Reason)
	}
	if maxTokensSent == nil || *maxTokensSent != compact.MaxTokensEscalation {
		var got string
		if maxTokensSent != nil {
			got = fmt.Sprintf("%d", *maxTokensSent)
		} else {
			got = "nil"
		}
		t.Fatalf("expected MaxTokens to be escalated to %d, got %v", compact.MaxTokensEscalation, got)
	}
}
