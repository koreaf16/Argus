package query

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/yourorg/infractl/internal/llm"
)

func researchMetaJSON(reason string, followUp bool) string {
	b, _ := json.Marshal(map[string]any{
		"completion_reason": reason,
		"follow_up_required": followUp,
	})
	return string(b)
}

func TestAutoContinue_FiresOnNoResults(t *testing.T) {
	s := state{researchAutoContinueCount: 0, lastResearchMetadata: researchMetaJSON("no_results", false)}
	if !shouldResearchAutoContinue(s, llm.Response{Content: "done"}) {
		t.Fatal("expected auto-continue for no_results metadata")
	}
}

func TestAutoContinue_FiresOnFollowUpRequired(t *testing.T) {
	s := state{researchAutoContinueCount: 0, lastResearchMetadata: researchMetaJSON("assistant_complete", true)}
	if !shouldResearchAutoContinue(s, llm.Response{Content: "done"}) {
		t.Fatal("expected auto-continue when follow_up_required=true")
	}
}

func TestAutoContinue_CappedAtTwo(t *testing.T) {
	s := state{researchAutoContinueCount: 2, lastResearchMetadata: researchMetaJSON("no_results", true)}
	if shouldResearchAutoContinue(s, llm.Response{Content: "done"}) {
		t.Fatal("auto-continue should not fire once cap is reached")
	}
}

func TestAutoContinue_DoesNotFireWhenMetadataEmpty(t *testing.T) {
	s := state{}
	if shouldResearchAutoContinue(s, llm.Response{Content: "done"}) {
		t.Fatal("expected no auto-continue without metadata")
	}
}

func TestAutoContinue_ResetOnNonResearchTool(t *testing.T) {
	client := &scenarioClient{turns: []llm.Response{toolCallTurn(gtc("1", "shell_exec"))}}
	eng := New(nil)
	se := NewStreamingExecutor(1)
	out := make(chan QueryEvent, 8)

	s := initialState(nil)
	s.researchAutoContinueCount = 1
	s.lastResearchMetadata = researchMetaJSON("no_results", true)

	done, next := eng.runTurn(context.Background(), Params{
		Client:    client,
		ModelName: "test",
		Tier:      llm.TierGeneral,
		SystemMsg: llm.Message{Role: llm.RoleSystem, Content: "test"},
		RunTool: func(context.Context, llm.ToolCall) (string, bool, string) {
			return "ok", false, ""
		},
	}, s, se, out)

	if done {
		t.Fatal("turn should continue after tool execution")
	}
	if next.researchAutoContinueCount != 0 {
		t.Fatalf("research auto-continue counter = %d, want 0", next.researchAutoContinueCount)
	}
}

func TestAutoContinue_IdenticalGuardStillFiresOnRealDuplicates(t *testing.T) {
	tc := gtc("1", "shell_exec")
	client := &scenarioClient{turns: []llm.Response{{ToolCalls: []llm.ToolCall{tc}}}}
	eng := New(nil)
	se := NewStreamingExecutor(1)
	out := make(chan QueryEvent, 8)

	s := initialState(nil)
	s.lastToolCalls = []llm.ToolCall{tc}
	s.lastResearchMetadata = researchMetaJSON("no_results", true)

	done, next := eng.runTurn(context.Background(), Params{
		Client:    client,
		ModelName: "test",
		Tier:      llm.TierGeneral,
		SystemMsg: llm.Message{Role: llm.RoleSystem, Content: "test"},
		RunTool: func(context.Context, llm.ToolCall) (string, bool, string) {
			return "ok", false, ""
		},
	}, s, se, out)

	if done {
		t.Fatal("duplicate guard path should continue, not finish")
	}
	if next.transition == nil || *next.transition != ContinueNextTurn {
		t.Fatalf("expected ContinueNextTurn transition, got %#v", next.transition)
	}
}
