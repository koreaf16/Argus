package llm

import "testing"

func TestAnthropicStreamDecoderAccumulatesToolInput(t *testing.T) {
	t.Parallel()

	decoder := newAnthropicStreamDecoder()

	// content_block_start에서 stall 방지용 EventToolUseDelta 하나를 emit해야 함
	if events, ok := decoder.Decode(`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"bash","input":{}}}`); !ok || len(events) != 1 || events[0].Kind != EventToolUseDelta {
		t.Fatalf("tool start should emit one EventToolUseDelta, got ok=%v events=%+v", ok, events)
	}
	if events, ok := decoder.Decode(`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"command\":\"pw"}}`); ok || len(events) != 0 {
		t.Fatalf("tool delta should be buffered, got ok=%v events=%+v", ok, events)
	}
	if events, ok := decoder.Decode(`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"d\"}"}}`); ok || len(events) != 0 {
		t.Fatalf("tool delta should be buffered, got ok=%v events=%+v", ok, events)
	}

	events, ok := decoder.Decode(`{"type":"content_block_stop","index":0}`)
	if !ok || len(events) != 1 {
		t.Fatalf("expected one tool event, got ok=%v events=%+v", ok, events)
	}
	if events[0].Kind != EventToolUseStart || events[0].ToolUse == nil {
		t.Fatalf("expected tool use event, got %+v", events[0])
	}
	if events[0].ToolUse.ID != "toolu_1" || events[0].ToolUse.Name != "bash" {
		t.Fatalf("unexpected tool metadata: %+v", events[0].ToolUse)
	}
	if string(events[0].ToolUse.Input) != `{"command":"pwd"}` {
		t.Fatalf("unexpected tool input: %s", string(events[0].ToolUse.Input))
	}
}

func TestAnthropicStreamDecoderKeepsTextDeltas(t *testing.T) {
	t.Parallel()

	decoder := newAnthropicStreamDecoder()
	events, ok := decoder.Decode(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`)
	if !ok || len(events) != 1 {
		t.Fatalf("expected one text event, got ok=%v events=%+v", ok, events)
	}
	if events[0].Kind != EventTextDelta || events[0].Delta != "hello" {
		t.Fatalf("unexpected text event: %+v", events[0])
	}
}

func TestAnthropicStreamDecoder_MessageStartEmitsUsage(t *testing.T) {
	t.Parallel()

	decoder := newAnthropicStreamDecoder()
	raw := `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1500,"output_tokens":0,"cache_creation_input_tokens":1200,"cache_read_input_tokens":0}}}`
	events, ok := decoder.Decode(raw)
	if !ok || len(events) != 1 {
		t.Fatalf("expected one usage event, got ok=%v events=%+v", ok, events)
	}
	if events[0].Kind != EventUsage {
		t.Fatalf("expected EventUsage, got %s", events[0].Kind)
	}
	u := events[0].Usage
	if u == nil {
		t.Fatal("expected non-nil usage")
	}
	if u.InputTokens != 1500 {
		t.Errorf("expected InputTokens=1500, got %d", u.InputTokens)
	}
	if u.CacheCreationInputTokens != 1200 {
		t.Errorf("expected CacheCreationInputTokens=1200, got %d", u.CacheCreationInputTokens)
	}
	if u.CacheReadInputTokens != 0 {
		t.Errorf("expected CacheReadInputTokens=0, got %d", u.CacheReadInputTokens)
	}
}

func TestAnthropicStreamDecoder_MessageDeltaEmitsCacheRead(t *testing.T) {
	t.Parallel()

	// Simulate a second-turn response where cache was hit (cache_read > 0, creation = 0).
	decoder := newAnthropicStreamDecoder()
	raw := `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":42,"cache_creation_input_tokens":0,"cache_read_input_tokens":1200}}`
	events, ok := decoder.Decode(raw)
	if !ok {
		t.Fatalf("expected events, got ok=false")
	}
	// Should get [EventUsage, EventStop] in order.
	if len(events) != 2 {
		t.Fatalf("expected 2 events (usage + stop), got %d: %+v", len(events), events)
	}
	if events[0].Kind != EventUsage {
		t.Errorf("first event must be EventUsage, got %s", events[0].Kind)
	}
	u := events[0].Usage
	if u == nil || u.CacheReadInputTokens != 1200 {
		t.Errorf("expected CacheReadInputTokens=1200, got %+v", u)
	}
	if u.CacheCreationInputTokens != 0 {
		t.Errorf("expected CacheCreationInputTokens=0 on cache-hit turn, got %d", u.CacheCreationInputTokens)
	}
	if events[1].Kind != EventStop {
		t.Errorf("second event must be EventStop, got %s", events[1].Kind)
	}
}
