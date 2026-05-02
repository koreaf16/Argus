package tools

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/koreaf16/argus/internal/services/llm"
	toolpkg "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/enterplanmode"
	"github.com/koreaf16/argus/internal/tools/exitplanmode"
	"github.com/koreaf16/argus/internal/tools/taskplaninit"
)

type orchestrationTestTool struct {
	name     string
	readOnly bool
	safeFn   func(input json.RawMessage) bool
}

func (t *orchestrationTestTool) Name() string { return t.name }

func (t *orchestrationTestTool) Description(ctx toolpkg.Context) string { return t.name }

func (t *orchestrationTestTool) InputSchema() toolpkg.ToolInputJSONSchema {
	return toolpkg.ToolInputJSONSchema{"type": "object"}
}

func (t *orchestrationTestTool) IsReadOnly() bool { return t.readOnly }

func (t *orchestrationTestTool) IsConcurrencySafe(input json.RawMessage) bool {
	if t.safeFn != nil {
		return t.safeFn(input)
	}
	return t.readOnly
}

func (t *orchestrationTestTool) Call(ctx toolpkg.Context, input json.RawMessage) (<-chan toolpkg.ToolEvent, error) {
	ch := make(chan toolpkg.ToolEvent)
	close(ch)
	return ch, nil
}

func (t *orchestrationTestTool) CheckPermission(ctx toolpkg.Context, input json.RawMessage) (toolpkg.PermissionResult, error) {
	return toolpkg.DefaultAllowPermission(), nil
}

func (t *orchestrationTestTool) MaxResultSizeChars() int { return 1000 }

func TestPartitionToolCalls_InputAwareSafety(t *testing.T) {
	t.Parallel()

	reg := toolpkg.NewRegistry()
	if err := reg.Register(&orchestrationTestTool{
		name:     "bash",
		readOnly: false,
		safeFn: func(input json.RawMessage) bool {
			cmd := strings.TrimSpace(toolpkg.ExtractStringInput(input, "command"))
			return strings.HasPrefix(cmd, "rg ")
		},
	}); err != nil {
		t.Fatalf("register bash: %v", err)
	}
	if err := reg.Register(&orchestrationTestTool{
		name:     "fileread",
		readOnly: true,
	}); err != nil {
		t.Fatalf("register fileread: %v", err)
	}

	calls := []llm.ToolUseStart{
		{ID: "1", Name: "bash", Input: json.RawMessage(`{"command":"rg foo ."}`)},
		{ID: "2", Name: "bash", Input: json.RawMessage(`{"command":"rg bar ."}`)},
		{ID: "3", Name: "bash", Input: json.RawMessage(`{"command":"echo hi > out.txt"}`)},
		{ID: "4", Name: "fileread", Input: json.RawMessage(`{"path":"README.md"}`)},
		{ID: "5", Name: "bash", Input: json.RawMessage(`{"command":"rg baz ."}`)},
	}

	batches := PartitionToolCalls(calls, reg)
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches, got %d", len(batches))
	}
	if !batches[0].IsConcurrencySafe || len(batches[0].Calls) != 2 {
		t.Fatalf("unexpected first batch: %+v", batches[0])
	}
	if batches[1].IsConcurrencySafe || len(batches[1].Calls) != 1 || batches[1].Calls[0].ID != "3" {
		t.Fatalf("unexpected second batch: %+v", batches[1])
	}
	if !batches[2].IsConcurrencySafe || len(batches[2].Calls) != 2 {
		t.Fatalf("unexpected third batch: %+v", batches[2])
	}
}

func TestPartitionToolCalls_InvalidInputAndPanicAreUnsafe(t *testing.T) {
	t.Parallel()

	reg := toolpkg.NewRegistry()
	if err := reg.Register(&orchestrationTestTool{
		name:     "panic_tool",
		readOnly: true,
		safeFn: func(input json.RawMessage) bool {
			panic("boom")
		},
	}); err != nil {
		t.Fatalf("register panic_tool: %v", err)
	}
	if err := reg.Register(&orchestrationTestTool{name: "read_tool", readOnly: true}); err != nil {
		t.Fatalf("register read_tool: %v", err)
	}

	batches := PartitionToolCalls([]llm.ToolUseStart{
		{ID: "panic", Name: "panic_tool", Input: json.RawMessage(`{}`)},
		{ID: "invalid", Name: "read_tool", Input: json.RawMessage(`{"unterminated"`)},
	}, reg)
	if len(batches) != 2 {
		t.Fatalf("expected 2 unsafe batches, got %d", len(batches))
	}
	for _, batch := range batches {
		if batch.IsConcurrencySafe {
			t.Fatalf("expected unsafe batch: %+v", batch)
		}
	}
}

func TestPartitionToolCalls_StateChangingPlanToolsAreUnsafe(t *testing.T) {
	t.Parallel()

	reg := toolpkg.NewRegistry()
	for _, tl := range []toolpkg.Tool{
		taskplaninit.New(),
		enterplanmode.NewEnterPlanModeTool(),
		exitplanmode.NewExitPlanModeTool(),
	} {
		if err := reg.Register(tl); err != nil {
			t.Fatalf("register %s: %v", tl.Name(), err)
		}
	}
	calls := []llm.ToolUseStart{
		{ID: "init", Name: taskplaninit.ToolName, Input: json.RawMessage(`{"title":"x"}`)},
		{ID: "enter", Name: "enter_plan_mode", Input: json.RawMessage(`{}`)},
		{ID: "exit", Name: "exit_plan_mode", Input: json.RawMessage(`{}`)},
	}
	batches := PartitionToolCalls(calls, reg)
	if len(batches) != len(calls) {
		t.Fatalf("expected each tool to be its own unsafe batch, got %+v", batches)
	}
	for _, batch := range batches {
		if batch.IsConcurrencySafe {
			t.Fatalf("state-changing plan tool was marked safe: %+v", batch)
		}
	}
}

func TestRunTools_ParallelBatchHonorsConcurrencyCap(t *testing.T) {
	t.Setenv("ARGUS_MAX_TOOL_CONCURRENCY", "2")

	reg := toolpkg.NewRegistry()
	if err := reg.Register(&orchestrationTestTool{name: "fileread", readOnly: true}); err != nil {
		t.Fatalf("register fileread: %v", err)
	}

	calls := []llm.ToolUseStart{
		{ID: "a", Name: "fileread"},
		{ID: "b", Name: "fileread"},
		{ID: "c", Name: "fileread"},
		{ID: "d", Name: "fileread"},
	}
	var running int32
	var maxRunning int32
	RunTools(context.Background(), calls, reg, nil, func(_ context.Context, call llm.ToolUseStart) (string, bool) {
		now := atomic.AddInt32(&running, 1)
		for {
			old := atomic.LoadInt32(&maxRunning)
			if now <= old || atomic.CompareAndSwapInt32(&maxRunning, old, now) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&running, -1)
		return call.ID, false
	})

	if got := atomic.LoadInt32(&maxRunning); got > 2 {
		t.Fatalf("max concurrency = %d, want <= 2", got)
	}
}

func TestRunTools_ParallelBatchPreservesOrder(t *testing.T) {
	t.Parallel()

	reg := toolpkg.NewRegistry()
	if err := reg.Register(&orchestrationTestTool{name: "fileread", readOnly: true}); err != nil {
		t.Fatalf("register fileread: %v", err)
	}

	calls := []llm.ToolUseStart{
		{ID: "a", Name: "fileread"},
		{ID: "b", Name: "fileread"},
		{ID: "c", Name: "fileread"},
	}

	results := RunTools(context.Background(), calls, reg, nil, func(_ context.Context, call llm.ToolUseStart) (string, bool) {
		switch call.ID {
		case "a":
			time.Sleep(25 * time.Millisecond)
		case "b":
			time.Sleep(5 * time.Millisecond)
		case "c":
			time.Sleep(15 * time.Millisecond)
		}
		return call.ID, false
	})

	if len(results) != len(calls) {
		t.Fatalf("expected %d results, got %d", len(calls), len(results))
	}
	for i := range calls {
		if results[i].Call.ID != calls[i].ID {
			t.Fatalf("result order mismatch at %d: got %s want %s", i, results[i].Call.ID, calls[i].ID)
		}
	}
}

func TestStreamingToolExecutorStartsBeforeCloseAndPreservesOrder(t *testing.T) {
	t.Parallel()

	reg := toolpkg.NewRegistry()
	if err := reg.Register(&orchestrationTestTool{name: "read", readOnly: true}); err != nil {
		t.Fatalf("register read: %v", err)
	}
	started := make(chan string, 2)
	exec := NewStreamingToolExecutor(context.Background(), reg, nil, nil, func(_ context.Context, call llm.ToolUseStart) (string, bool) {
		started <- call.ID
		if call.ID == "a" {
			time.Sleep(25 * time.Millisecond)
		}
		return call.ID, false
	})

	exec.Add(llm.ToolUseStart{ID: "a", Name: "read"})
	select {
	case got := <-started:
		if got != "a" {
			t.Fatalf("unexpected started call: %s", got)
		}
	case <-time.After(time.Second):
		t.Fatal("tool did not start before CloseAndWait")
	}
	exec.Add(llm.ToolUseStart{ID: "b", Name: "read"})
	results := exec.CloseAndWait()
	if len(results) != 2 || results[0].Call.ID != "a" || results[1].Call.ID != "b" {
		t.Fatalf("unexpected ordered results: %+v", results)
	}
}

func TestStreamingToolExecutorUnsafeToolBlocksLaterTools(t *testing.T) {
	t.Parallel()

	reg := toolpkg.NewRegistry()
	if err := reg.Register(&orchestrationTestTool{name: "read", readOnly: true}); err != nil {
		t.Fatalf("register read: %v", err)
	}
	if err := reg.Register(&orchestrationTestTool{name: "write", readOnly: false}); err != nil {
		t.Fatalf("register write: %v", err)
	}
	started := make(chan string, 3)
	exec := NewStreamingToolExecutor(context.Background(), reg, nil, nil, func(_ context.Context, call llm.ToolUseStart) (string, bool) {
		started <- call.ID
		time.Sleep(10 * time.Millisecond)
		return call.ID, false
	})
	exec.Add(llm.ToolUseStart{ID: "a", Name: "read"})
	exec.Add(llm.ToolUseStart{ID: "b", Name: "write"})
	exec.Add(llm.ToolUseStart{ID: "c", Name: "read"})
	results := exec.CloseAndWait()
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	gotOrder := []string{<-started, <-started, <-started}
	wantOrder := []string{"a", "b", "c"}
	for i := range wantOrder {
		if gotOrder[i] != wantOrder[i] {
			t.Fatalf("start order = %v, want %v", gotOrder, wantOrder)
		}
	}
}
