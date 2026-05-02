package query

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/state"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
)

// findUIEvent returns true if the given UIEventKind appears in the slice.
func findUIEvent(kinds []UIEventKind, want UIEventKind) bool {
	for _, k := range kinds {
		if k == want {
			return true
		}
	}
	return false
}

// findTrace returns the first TraceRecord with the given type, or the zero value.
func findTrace(records []TraceRecord, wantType string) (TraceRecord, bool) {
	for _, r := range records {
		if r.Type == wantType {
			return r, true
		}
	}
	return TraceRecord{}, false
}

// requireUIEvent fails if UIEventKind is absent from the collected events.
func requireUIEvent(t *testing.T, kinds []UIEventKind, want UIEventKind) {
	t.Helper()
	if !findUIEvent(kinds, want) {
		t.Errorf("UIEvent %q not found in event stream", want)
	}
}

// requireTrace fails if the trace type is absent and returns the first matching record.
func requireTrace(t *testing.T, records []TraceRecord, wantType string) TraceRecord {
	t.Helper()
	r, ok := findTrace(records, wantType)
	if !ok {
		t.Errorf("trace type %q not found in trace records (got: %d records)", wantType, len(records))
	}
	return r
}

func newAIDebugEngine(client llm.LLM, reg *tool.Registry) (*Engine, *traceCaptureEmitter) {
	emitter := &traceCaptureEmitter{}
	engine := NewEngine(client, reg, state.NewAppState(), nil)
	engine.SetDeps(context.Background(), Deps{
		WorkingDir: ".",
		SessionID:  "equivalence-test",
		AIDebug: AIDebugConfig{
			Enabled: true,
			Emitter: emitter,
		},
	})
	return engine, emitter
}

func collectEvents(ch <-chan UIEvent) []UIEventKind {
	var kinds []UIEventKind
	for evt := range ch {
		kinds = append(kinds, evt.Kind)
	}
	return kinds
}

// TestAIDebugEquivalence_TextResponse는 텍스트 응답 턴에서
// UIEvent 채널과 aidebug trace 채널의 등가성을 검증합니다.
func TestAIDebugEquivalence_TextResponse(t *testing.T) {
	engine, emitter := newAIDebugEngine(&fakeTraceLLM{}, nil)

	ch, err := engine.SubmitMessage(context.Background(), "hello equivalence")
	if err != nil {
		t.Fatalf("SubmitMessage: %v", err)
	}
	kinds := collectEvents(ch)
	traces := emitter.list()

	// UIEventAssistantDelta → llm.provider_chunk 존재해야 함
	requireUIEvent(t, kinds, UIEventAssistantDelta)
	requireTrace(t, traces, "llm.provider_chunk")

	// UIEventDone → turn.finish 존재해야 하며 stop_reason 포함
	requireUIEvent(t, kinds, UIEventDone)
	finish := requireTrace(t, traces, "turn.finish")
	if data, ok := finish.Data.(map[string]any); ok {
		if _, hasStop := data["stop_reason"]; !hasStop {
			t.Error("turn.finish trace에 stop_reason 필드가 없습니다")
		}
	}

	// turn.start → turn.finish 순서 보장 (Seq 기준)
	start := requireTrace(t, traces, "turn.start")
	if start.Seq > 0 && finish.Seq > 0 && start.Seq >= finish.Seq {
		t.Errorf("turn.start seq(%d) should be < turn.finish seq(%d)", start.Seq, finish.Seq)
	}

	// 모든 trace record의 turn_index가 0보다 커야 함
	for _, r := range traces {
		if r.Type == "session.start" {
			continue // session.start는 turn_index가 0이어도 OK
		}
		if r.TurnIndex <= 0 {
			t.Errorf("trace %q의 turn_index=%d, 1 이상이어야 합니다", r.Type, r.TurnIndex)
		}
	}

	// llm.request와 llm.provider_chunk가 모두 발행되어야 함
	requireTrace(t, traces, "llm.request")
}

// TestAIDebugEquivalence_ToolUse는 도구 호출 턴에서
// UIEvent 채널과 aidebug trace 채널의 등가성을 검증합니다.
func TestAIDebugEquivalence_ToolUse(t *testing.T) {
	stopTool := llm.StopReasonToolUse
	stopEnd := llm.StopReasonEndTurn

	client := &scriptedLLM{
		sequences: [][]llm.Event{
			{
				{Kind: llm.EventToolUseStart, ToolUse: &llm.ToolUseStart{
					ID:    "eq-t1",
					Name:  "eq_tool",
					Input: json.RawMessage(`{"key":"value"}`),
				}},
				{Kind: llm.EventStop, Stop: &stopTool},
			},
			{
				{Kind: llm.EventTextDelta, Delta: "all done"},
				{Kind: llm.EventStop, Stop: &stopEnd},
			},
		},
	}

	reg := tool.NewRegistry()
	if err := reg.Register(&scriptedTool{
		name:       "eq_tool",
		permission: types.BehaviorAllow,
		output:     "tool output for equivalence test",
	}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	engine, emitter := newAIDebugEngine(client, reg)
	cfg := DefaultConfig()
	cfg.MaxToolIterations = 4
	engine.SetConfig(cfg)

	ch, err := engine.SubmitMessage(context.Background(), "run tool")
	if err != nil {
		t.Fatalf("SubmitMessage: %v", err)
	}
	kinds := collectEvents(ch)
	traces := emitter.list()

	// UIEventToolUse → tool.call.start
	requireUIEvent(t, kinds, UIEventToolUse)
	toolStart := requireTrace(t, traces, "tool.call.start")
	if data, ok := toolStart.Data.(map[string]any); ok {
		if name, _ := data["tool"].(string); name != "eq_tool" {
			t.Errorf("tool.call.start tool=%q, want %q", name, "eq_tool")
		}
	}

	// UIEventToolResult → tool.call.output
	requireUIEvent(t, kinds, UIEventToolResult)
	requireTrace(t, traces, "tool.call.output")

	// 도구 실행 notice → notice trace
	requireUIEvent(t, kinds, UIEventNotice)
	requireTrace(t, traces, "notice")

	// tool.call.start가 tool.call.finish보다 먼저 나와야 함
	callStart, hasStart := findTrace(traces, "tool.call.start")
	callFinish, hasFinish := findTrace(traces, "tool.call.finish")
	if hasStart && hasFinish && callStart.Seq >= callFinish.Seq {
		t.Errorf("tool.call.start seq(%d) should be < tool.call.finish seq(%d)", callStart.Seq, callFinish.Seq)
	}

	// turn.finish에 stop_reason 포함
	finish := requireTrace(t, traces, "turn.finish")
	if data, ok := finish.Data.(map[string]any); ok {
		if _, hasStop := data["stop_reason"]; !hasStop {
			t.Error("tool use 턴의 turn.finish trace에 stop_reason이 없습니다")
		}
	}
}

// TestAIDebugEquivalence_TraceCompleteness는 기본 턴에서
// 최소한의 필수 trace type 집합이 모두 발행되는지 검증합니다.
func TestAIDebugEquivalence_TraceCompleteness(t *testing.T) {
	engine, emitter := newAIDebugEngine(&fakeTraceLLM{}, nil)
	ch, err := engine.SubmitMessage(context.Background(), "completeness check")
	if err != nil {
		t.Fatalf("SubmitMessage: %v", err)
	}
	for range ch {
	}

	traces := emitter.list()
	traceTypeSet := make(map[string]struct{}, len(traces))
	for _, r := range traces {
		traceTypeSet[r.Type] = struct{}{}
	}

	// 텍스트 응답 턴에서 반드시 있어야 하는 trace type들
	required := []string{
		"session.start",
		"turn.start",
		"llm.request",
		"llm.provider_chunk",
		"assistant.final",
		"turn.finish",
	}
	for _, typ := range required {
		if _, ok := traceTypeSet[typ]; !ok {
			t.Errorf("필수 trace %q가 발행되지 않았습니다", typ)
		}
	}
}
