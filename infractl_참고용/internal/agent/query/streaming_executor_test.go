package query

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yourorg/infractl/internal/llm"
)

func makeToolCall(id, name string) llm.ToolCall {
	return llm.ToolCall{ID: id, Type: "function", Function: llm.FunctionCall{Name: name, Arguments: "{}"}}
}

func echoRunner(_ context.Context, tc llm.ToolCall) (string, bool, string) {
	return "output:" + tc.Function.Name, false, `{"kind":"test"}`
}

func errorRunner(_ context.Context, tc llm.ToolCall) (string, bool, string) {
	return "error from " + tc.Function.Name, true, ""
}

func drainEvents(ch <-chan QueryEvent) []QueryEvent {
	var evs []QueryEvent
	for ev := range ch {
		evs = append(evs, ev)
	}
	return evs
}

func TestStreamingExecutor_ConcurrentBatch_AllComplete(t *testing.T) {
	se := NewStreamingExecutor(10)
	calls := []llm.ToolCall{makeToolCall("1", "read1"), makeToolCall("2", "read2"), makeToolCall("3", "read3")}
	batches := []Batch{{Concurrent: true, Calls: calls}}

	out := make(chan QueryEvent, 32)
	results, infos, aborted := se.Execute(context.Background(), batches, out, echoRunner)
	close(out)

	if aborted {
		t.Error("should not be aborted")
	}
	if len(results) != 3 {
		t.Errorf("want 3 results, got %d", len(results))
	}
	if len(infos) != 3 {
		t.Fatalf("want 3 infos, got %d", len(infos))
	}
	for _, info := range infos {
		if info.MetadataJSON == "" {
			t.Fatalf("expected metadata to round-trip")
		}
	}

	events := drainEvents(out)
	var toolResults []EventToolResult
	for _, ev := range events {
		if r, ok := ev.(EventToolResult); ok {
			toolResults = append(toolResults, r)
		}
	}
	if len(toolResults) != 3 {
		t.Errorf("want 3 EventToolResult events, got %d", len(toolResults))
	}
	for _, r := range toolResults {
		if r.MetadataJSON == "" {
			t.Fatalf("event metadata should be populated")
		}
	}
	for _, m := range results {
		if !strings.HasPrefix(m.Content, "output:") {
			t.Errorf("unexpected result content: %q", m.Content)
		}
	}
}

func TestStreamingExecutor_SerialBatch_SiblingAbort(t *testing.T) {
	se := NewStreamingExecutor(10)
	calls := []llm.ToolCall{makeToolCall("1", "mut1"), makeToolCall("2", "mut2"), makeToolCall("3", "mut3")}
	batches := []Batch{{Concurrent: false, Calls: calls}}

	out := make(chan QueryEvent, 32)
	results, infos, aborted := se.Execute(context.Background(), batches, out, errorRunner)
	close(out)

	if aborted {
		t.Error("should not be aborted")
	}
	if len(results) != 3 {
		t.Fatalf("want 3 results, got %d", len(results))
	}
	if len(infos) != 3 {
		t.Fatalf("want 3 infos, got %d", len(infos))
	}

	events := drainEvents(out)
	var skipped []EventToolResult
	for _, ev := range events {
		if r, ok := ev.(EventToolResult); ok && r.SiblingSkipped {
			skipped = append(skipped, r)
		}
	}
	if len(skipped) != 2 {
		t.Errorf("want 2 sibling-skipped events, got %d", len(skipped))
	}
	for _, r := range skipped {
		if !strings.Contains(r.Output, "sibling failure") {
			t.Errorf("sibling skip message should mention sibling failure: %q", r.Output)
		}
	}
}

func TestStreamingExecutor_CtxCancel_Aborted(t *testing.T) {
	se := NewStreamingExecutor(1)

	var started atomic.Int32
	slowRunner := func(ctx context.Context, tc llm.ToolCall) (string, bool, string) {
		started.Add(1)
		select {
		case <-ctx.Done():
			return "cancelled", true, ""
		case <-time.After(10 * time.Second):
			return "slow", false, ""
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	calls := []llm.ToolCall{makeToolCall("1", "slow")}
	batches := []Batch{{Concurrent: true, Calls: calls}}

	out := make(chan QueryEvent, 32)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = se.Execute(ctx, batches, out, slowRunner)
	}()

	for started.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
}

func TestStreamingExecutor_MultiBatch_OrderPreserved(t *testing.T) {
	se := NewStreamingExecutor(10)
	calls := []llm.ToolCall{makeToolCall("1", "r1"), makeToolCall("2", "r2"), makeToolCall("3", "m1"), makeToolCall("4", "r3")}
	batches := []Batch{{Concurrent: true, Calls: calls[:2]}, {Concurrent: false, Calls: calls[2:3]}, {Concurrent: true, Calls: calls[3:]}}

	out := make(chan QueryEvent, 32)
	results, infos, aborted := se.Execute(context.Background(), batches, out, echoRunner)
	close(out)

	if aborted {
		t.Error("should not be aborted")
	}
	if len(results) != 4 {
		t.Fatalf("want 4 results, got %d", len(results))
	}
	if len(infos) != 4 {
		t.Fatalf("want 4 infos, got %d", len(infos))
	}
	for i, m := range results {
		if m.ToolCallID != calls[i].ID {
			t.Errorf("result[%d]: want ToolCallID=%q, got %q", i, calls[i].ID, m.ToolCallID)
		}
	}
}
