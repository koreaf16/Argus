package query

import (
	"context"
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/llm"
)

func TestRunLoop_EmptyResponseTwiceReturnsModelError(t *testing.T) {
	client := &scenarioClient{
		turns: []llm.Response{
			{}, // first empty response -> retry
			{}, // second empty response -> terminal model error
		},
	}

	evs := collectEvents(New(nil).Run(context.Background(), Params{
		SystemMsg: llm.Message{Role: llm.RoleSystem, Content: "sys"},
		Tier:      llm.TierGeneral,
		Client:    client,
		ModelName: "test-model",
		MaxTurns:  5,
	}))

	term := findTerminal(t, evs)
	if term.Reason != TerminalModelError {
		t.Fatalf("terminal reason = %q, want %q", term.Reason, TerminalModelError)
	}
	if term.Err == nil {
		t.Fatal("expected terminal error for repeated empty responses")
	}
	if !strings.Contains(term.Err.Error(), "empty response") {
		t.Fatalf("terminal error = %v, want empty response hint", term.Err)
	}
}
