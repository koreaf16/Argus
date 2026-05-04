package query

import (
	"context"
	"strings"
	"testing"

	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/state"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
)

type fakeThinkingLLM struct {
	events []llm.Event
}

func (f *fakeThinkingLLM) Stream(context.Context, llm.Request) (<-chan llm.Event, error) {
	ch := make(chan llm.Event, len(f.events))
	for _, evt := range f.events {
		ch <- evt
	}
	close(ch)
	return ch, nil
}

func (f *fakeThinkingLLM) CountTokens(context.Context, llm.Request) (int, error) {
	return 20, nil
}

func (f *fakeThinkingLLM) Capabilities() llm.Caps {
	return llm.Caps{Thinking: true}
}

func (f *fakeThinkingLLM) Provider() string {
	return "fake"
}

func TestSubmitMessage_ThinkingLifecycle(t *testing.T) {
	stop := llm.StopReasonEndTurn
	fake := &fakeThinkingLLM{
		events: []llm.Event{
			{Kind: llm.EventThinkingDelta, Delta: "Thinking..."},
			{Kind: llm.EventTextDelta, Delta: "Hello"},
			{Kind: llm.EventStop, Stop: &stop},
		},
	}

	engine := NewEngine(fake, nil, state.NewAppState(), nil)
	cfg := DefaultConfig()
	cfg.MaxToolIterations = 1
	engine.SetConfig(cfg)

	ch, err := engine.SubmitMessage(context.Background(), "hi")
	if err != nil {
		t.Fatalf("submit message failed: %v", err)
	}

	var kinds []UIEventKind
	for evt := range ch {
		kinds = append(kinds, evt.Kind)
	}

	if len(kinds) < 4 {
		t.Fatalf("expected at least 4 events, got %d (%v)", len(kinds), kinds)
	}
	if kinds[0] != UIEventThinkingDelta {
		t.Fatalf("expected first event thinking delta, got %s", kinds[0])
	}
	if kinds[1] != UIEventThinkingDone {
		t.Fatalf("expected second event thinking done, got %s", kinds[1])
	}
	if kinds[2] != UIEventAssistantDelta {
		t.Fatalf("expected third event assistant delta, got %s", kinds[2])
	}
	if kinds[len(kinds)-1] != UIEventDone {
		t.Fatalf("expected final event done, got %s", kinds[len(kinds)-1])
	}
}

func TestSubmitMessage_NoLLM_DoesNotMutateMessages(t *testing.T) {
	engine := NewEngine(nil, nil, state.NewAppState(), nil)
	if len(engine.Messages()) != 0 {
		t.Fatalf("expected empty messages at start")
	}

	_, err := engine.SubmitMessage(context.Background(), "hello")
	if err == nil {
		t.Fatalf("expected error when llm is not configured")
	}
	if len(engine.Messages()) != 0 {
		t.Fatalf("expected messages to remain unchanged on llm-not-configured")
	}
}

func TestSubmitMessage_GreetingDoesNotForceToolRetry(t *testing.T) {
	stop := llm.StopReasonEndTurn
	client := &scriptedLLM{
		sequences: [][]llm.Event{
			{
				{Kind: llm.EventTextDelta, Delta: "안녕하세요! 무엇을 도와드릴까요?"},
				{Kind: llm.EventStop, Stop: &stop},
			},
			{
				{Kind: llm.EventTextDelta, Delta: "forced retry"},
				{Kind: llm.EventStop, Stop: &stop},
			},
		},
	}

	reg := tool.NewRegistry()
	if err := reg.Register(&scriptedTool{name: "noop", permission: types.BehaviorAllow}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	engine := NewEngine(client, reg, state.NewAppState(), nil)
	cfg := DefaultConfig()
	cfg.MaxToolIterations = 3
	engine.SetConfig(cfg)

	ch, err := engine.SubmitMessage(context.Background(), "하이")
	if err != nil {
		t.Fatalf("submit message failed: %v", err)
	}

	var text strings.Builder
	for evt := range ch {
		if evt.Kind == UIEventError && evt.Err != nil {
			t.Fatalf("unexpected error: %v", evt.Err)
		}
		if evt.Kind == UIEventAssistantDelta {
			text.WriteString(evt.Delta)
		}
	}

	client.mu.Lock()
	calls := client.index
	client.mu.Unlock()
	if calls != 1 {
		t.Fatalf("expected greeting to use one LLM call, got %d", calls)
	}
	if got := text.String(); got != "안녕하세요! 무엇을 도와드릴까요?" {
		t.Fatalf("unexpected assistant text: %q", got)
	}
}

func TestSubmitMessage_TaskRequestDoesNotForceToolRetryWhenTextOnly(t *testing.T) {
	stop := llm.StopReasonEndTurn
	client := &scriptedLLM{
		sequences: [][]llm.Event{
			{
				{Kind: llm.EventTextDelta, Delta: "텍스트로만 답변했습니다."},
				{Kind: llm.EventStop, Stop: &stop},
			},
		},
	}

	reg := tool.NewRegistry()
	if err := reg.Register(&scriptedTool{name: "noop", permission: types.BehaviorAllow}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	engine := NewEngine(client, reg, state.NewAppState(), nil)
	cfg := DefaultConfig()
	cfg.MaxToolIterations = 3
	engine.SetConfig(cfg)

	ch, err := engine.SubmitMessage(context.Background(), "현재 디렉터리 파일 목록 확인해줘")
	if err != nil {
		t.Fatalf("submit message failed: %v", err)
	}
	for evt := range ch {
		if evt.Kind == UIEventError && evt.Err != nil {
			t.Fatalf("unexpected error: %v", evt.Err)
		}
	}

	client.mu.Lock()
	calls := client.index
	client.mu.Unlock()
	if calls != 1 {
		t.Fatalf("expected task request text response to finish without forced retry, got %d LLM calls", calls)
	}
}
