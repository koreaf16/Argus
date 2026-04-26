package llm

import "testing"

func TestStreamAccumulatorOpenAI(t *testing.T) {
	t.Parallel()
	acc := NewStreamAccumulator()
	acc.AppendOpenAIToolDelta(0, "call_1", "search", `{"q":"he`)
	acc.AppendOpenAIToolDelta(0, "", "", `llo"}`)

	uses := acc.FlushOpenAIToolUses()
	if len(uses) != 1 {
		t.Fatalf("expected 1 tool use, got %d", len(uses))
	}
	if uses[0].ID != "call_1" || uses[0].Name != "search" {
		t.Fatalf("unexpected tool use: %+v", uses[0])
	}
	if string(uses[0].Input) != `{"q":"hello"}` {
		t.Fatalf("unexpected args: %s", string(uses[0].Input))
	}
}

func TestBuildToolResultMessage(t *testing.T) {
	t.Parallel()
	msg := BuildToolResultMessage("id1", "grep", "ok", false)
	if msg.Role != RoleUser || len(msg.Content) != 1 {
		t.Fatalf("unexpected message: %+v", msg)
	}
	if msg.Content[0].Type != ContentToolResult {
		t.Fatalf("unexpected content type: %s", msg.Content[0].Type)
	}
}

func TestStreamAccumulatorOpenAI_OrderByIndex(t *testing.T) {
	t.Parallel()

	acc := NewStreamAccumulator()
	acc.AppendOpenAIToolDelta(2, "call_3", "three", `{}`)
	acc.AppendOpenAIToolDelta(0, "call_1", "one", `{}`)
	acc.AppendOpenAIToolDelta(1, "call_2", "two", `{}`)

	uses := acc.FlushOpenAIToolUses()
	if len(uses) != 3 {
		t.Fatalf("expected 3 tool uses, got %d", len(uses))
	}

	if uses[0].ID != "call_1" || uses[1].ID != "call_2" || uses[2].ID != "call_3" {
		t.Fatalf("unexpected order: %+v", uses)
	}
}
