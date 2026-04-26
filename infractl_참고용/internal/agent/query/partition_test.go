package query

import (
	"context"
	"testing"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/tools"
)

type stubTool struct {
	name     string
	readOnly bool
}

func (s stubTool) Name() string               { return s.name }
func (s stubTool) Description() string        { return "" }
func (s stubTool) Parameters() map[string]any { return map[string]any{} }
func (s stubTool) IsReadOnly() bool           { return s.readOnly }
func (s stubTool) IsEnabled() bool            { return true }
func (s stubTool) Execute(context.Context, map[string]any, executor.Executor) (tools.ToolOutcome, error) {
	return tools.ToolOutcome{}, nil
}

type argAwareStubTool struct {
	stubTool
}

func (s argAwareStubTool) IsConcurrencySafe(args map[string]interface{}) bool {
	v, _ := args["safe"].(bool)
	return v
}

func makeRegistry(defs ...tools.Tool) *tools.Registry {
	r := tools.NewRegistry()
	for _, d := range defs {
		_ = r.Register(d)
	}
	return r
}

func tc(id, name string) llm.ToolCall {
	return llm.ToolCall{
		ID:   id,
		Type: "function",
		Function: llm.FunctionCall{
			Name:      name,
			Arguments: "{}",
		},
	}
}

func TestPartitionToolCalls_AllReadOnly(t *testing.T) {
	reg := makeRegistry(
		stubTool{name: "read1", readOnly: true},
		stubTool{name: "read2", readOnly: true},
		stubTool{name: "read3", readOnly: true},
	)
	calls := []llm.ToolCall{tc("1", "read1"), tc("2", "read2"), tc("3", "read3")}
	batches := PartitionToolCalls(calls, reg)

	if len(batches) != 1 {
		t.Fatalf("want 1 concurrent batch, got %d", len(batches))
	}
	if !batches[0].Concurrent {
		t.Fatalf("want first batch concurrent")
	}
	if len(batches[0].Calls) != 3 {
		t.Fatalf("want 3 calls in batch, got %d", len(batches[0].Calls))
	}
}

func TestPartitionToolCalls_AllMutation(t *testing.T) {
	reg := makeRegistry(
		stubTool{name: "mut1", readOnly: false},
		stubTool{name: "mut2", readOnly: false},
	)
	calls := []llm.ToolCall{tc("1", "mut1"), tc("2", "mut2")}
	batches := PartitionToolCalls(calls, reg)

	if len(batches) != 1 {
		t.Fatalf("want 1 serial batch, got %d", len(batches))
	}
	if batches[0].Concurrent {
		t.Fatalf("want mutation batch serial")
	}
	if len(batches[0].Calls) != 2 {
		t.Fatalf("want 2 calls in batch, got %d", len(batches[0].Calls))
	}
}

func TestPartitionToolCalls_Mixed(t *testing.T) {
	reg := makeRegistry(
		stubTool{name: "read1", readOnly: true},
		stubTool{name: "read2", readOnly: true},
		stubTool{name: "mut1", readOnly: false},
		stubTool{name: "read3", readOnly: true},
	)
	calls := []llm.ToolCall{
		tc("1", "read1"), tc("2", "read2"),
		tc("3", "mut1"),
		tc("4", "read3"),
	}
	batches := PartitionToolCalls(calls, reg)

	if len(batches) != 3 {
		t.Fatalf("want 3 batches, got %d", len(batches))
	}
	if !batches[0].Concurrent || len(batches[0].Calls) != 2 {
		t.Fatalf("batch 0 mismatch: %+v", batches[0])
	}
	if batches[1].Concurrent || len(batches[1].Calls) != 1 {
		t.Fatalf("batch 1 mismatch: %+v", batches[1])
	}
	if !batches[2].Concurrent || len(batches[2].Calls) != 1 {
		t.Fatalf("batch 2 mismatch: %+v", batches[2])
	}
}

func TestPartitionToolCalls_Empty(t *testing.T) {
	batches := PartitionToolCalls(nil, nil)
	if len(batches) != 0 {
		t.Fatalf("want no batches, got %d", len(batches))
	}
}

func TestPartitionToolCalls_UnknownTool(t *testing.T) {
	reg := makeRegistry()
	calls := []llm.ToolCall{tc("1", "unknown")}
	batches := PartitionToolCalls(calls, reg)

	if len(batches) != 1 || batches[0].Concurrent {
		t.Fatalf("unknown tool should be serial: %+v", batches)
	}
}

func TestPartitionToolCalls_UsesArgAwareConcurrencyCheck(t *testing.T) {
	reg := makeRegistry(argAwareStubTool{stubTool{name: "fetch", readOnly: false}})
	calls := []llm.ToolCall{
		{
			ID:   "1",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "fetch",
				Arguments: `{"safe":true}`,
			},
		},
		{
			ID:   "2",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "fetch",
				Arguments: `{"safe":false}`,
			},
		},
	}
	batches := PartitionToolCalls(calls, reg)

	if len(batches) != 2 {
		t.Fatalf("want 2 batches, got %d", len(batches))
	}
	if !batches[0].Concurrent {
		t.Fatalf("first batch should be concurrent")
	}
	if batches[1].Concurrent {
		t.Fatalf("second batch should be serial")
	}
}

func TestPartitionToolCalls_InvalidJSONFallsBackToSerial(t *testing.T) {
	reg := makeRegistry(argAwareStubTool{stubTool{name: "fetch", readOnly: true}})
	calls := []llm.ToolCall{
		{
			ID:   "1",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "fetch",
				Arguments: "{invalid",
			},
		},
	}
	batches := PartitionToolCalls(calls, reg)

	if len(batches) != 1 {
		t.Fatalf("want 1 batch, got %d", len(batches))
	}
	if batches[0].Concurrent {
		t.Fatalf("invalid args should be serial")
	}
}

func TestMaxToolConcurrency_Default(t *testing.T) {
	t.Setenv("INFRACTL_MAX_TOOL_CONCURRENCY", "")
	if got := MaxToolConcurrency(); got != defaultMaxConcurrency {
		t.Fatalf("want %d, got %d", defaultMaxConcurrency, got)
	}
}

func TestMaxToolConcurrency_EnvOverride(t *testing.T) {
	t.Setenv("INFRACTL_MAX_TOOL_CONCURRENCY", "5")
	if got := MaxToolConcurrency(); got != 5 {
		t.Fatalf("want 5, got %d", got)
	}
}

func TestMaxToolConcurrency_InvalidEnv(t *testing.T) {
	t.Setenv("INFRACTL_MAX_TOOL_CONCURRENCY", "abc")
	if got := MaxToolConcurrency(); got != defaultMaxConcurrency {
		t.Fatalf("want default %d, got %d", defaultMaxConcurrency, got)
	}
}
