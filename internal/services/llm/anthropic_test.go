package llm

import "testing"

func TestAnthropicStreamDecoderAccumulatesToolInput(t *testing.T) {
	t.Parallel()

	decoder := newAnthropicStreamDecoder()

	if events, ok := decoder.Decode(`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"bash","input":{}}}`); ok || len(events) != 0 {
		t.Fatalf("tool start should wait for JSON deltas, got ok=%v events=%+v", ok, events)
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
