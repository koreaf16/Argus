package query

import (
	"context"
	"strings"
	"testing"

	"github.com/koreaf16/argus/internal/services/llm"
)

func TestDefaultConfigMaxTokens(t *testing.T) {
	if got := DefaultConfig().MaxTokens; got != 4096 {
		t.Fatalf("DefaultConfig().MaxTokens = %d, want 4096", got)
	}
}

func TestAIDebugAssistantFinalNoticeOnMaxTokens(t *testing.T) {
	stop := llm.StopReasonMaxTokens
	client := &scriptedLLM{
		sequences: [][]llm.Event{
			{
				{Kind: llm.EventTextDelta, Delta: "partial response"},
				{Kind: llm.EventStop, Stop: &stop},
			},
		},
	}
	engine, emitter := newAIDebugEngine(client, nil)

	ch, err := engine.SubmitMessage(context.Background(), "trigger max token stop")
	if err != nil {
		t.Fatalf("SubmitMessage: %v", err)
	}

	var text strings.Builder
	done := false
	for evt := range ch {
		if evt.Kind == UIEventAssistantDelta {
			text.WriteString(evt.Delta)
		}
		if evt.Kind == UIEventDone {
			done = true
			if evt.StopReason != llm.StopReasonMaxTokens {
				t.Fatalf("UIEventDone stop = %q, want %q", evt.StopReason, llm.StopReasonMaxTokens)
			}
		}
	}
	if !done {
		t.Fatalf("expected UIEventDone")
	}
	if !strings.Contains(text.String(), "Output token limit reached") {
		t.Fatalf("assistant output missing max-token notice: %q", text.String())
	}

	final := requireTrace(t, emitter.list(), "assistant.final")
	data, ok := final.Data.(map[string]any)
	if !ok {
		t.Fatalf("assistant.final data = %T, want map[string]any", final.Data)
	}
	if got, _ := data["stop_reason"].(string); got != string(llm.StopReasonMaxTokens) {
		t.Fatalf("assistant.final stop_reason = %q, want %q", got, llm.StopReasonMaxTokens)
	}
	if got, _ := data["text"].(string); !strings.Contains(got, "Output token limit reached") {
		t.Fatalf("assistant.final text missing max-token notice: %q", got)
	}
}
