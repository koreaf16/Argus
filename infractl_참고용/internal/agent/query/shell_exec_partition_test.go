package query

import (
	"testing"

	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/tools"
)

func TestPartitionToolCalls_ShellExec(t *testing.T) {
	reg := tools.NewRegistry()
	_ = reg.Register(&tools.ShellExecTool{})

	calls := []llm.ToolCall{
		{
			ID:   "1",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "shell_exec",
				Arguments: `{"command":"ls -l"}`,
			},
		},
		{
			ID:   "2",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "shell_exec",
				Arguments: `{"command":"df -h"}`,
			},
		},
		{
			ID:   "3",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "shell_exec",
				Arguments: `{"command":"touch test.txt"}`,
			},
		},
	}

	batches := PartitionToolCalls(calls, reg)

	// Expected:
	// Batch 0: ["ls -l", "df -h"] (Concurrent=true)
	// Batch 1: ["touch test.txt"] (Concurrent=false)

	if len(batches) != 2 {
		t.Fatalf("want 2 batches, got %d", len(batches))
	}
	if !batches[0].Concurrent {
		t.Errorf("batch 0 should be concurrent (ls, df)")
	}
	if len(batches[0].Calls) != 2 {
		t.Errorf("batch 0 should have 2 calls, got %d", len(batches[0].Calls))
	}
	if batches[1].Concurrent {
		t.Errorf("batch 1 should be serial (touch)")
	}
}

func TestPartitionToolCalls_ShellExec_Redirection(t *testing.T) {
	reg := tools.NewRegistry()
	_ = reg.Register(&tools.ShellExecTool{})

	calls := []llm.ToolCall{
		{
			ID:   "1",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "shell_exec",
				Arguments: `{"command":"ls > out.txt"}`,
			},
		},
	}

	batches := PartitionToolCalls(calls, reg)

	if len(batches) != 1 {
		t.Fatalf("want 1 batch, got %d", len(batches))
	}
	if batches[0].Concurrent {
		t.Errorf("command with redirection should be serial")
	}
}

func TestPartitionToolCalls_ShellExec_Background(t *testing.T) {
	reg := tools.NewRegistry()
	_ = reg.Register(&tools.ShellExecTool{})

	calls := []llm.ToolCall{
		{
			ID:   "1",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "shell_exec",
				Arguments: `{"command":"long_running_cmd", "is_background":true}`,
			},
		},
		{
			ID:   "2",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "shell_exec",
				Arguments: `{"command":"another_bg_cmd", "is_background":true}`,
			},
		},
	}

	batches := PartitionToolCalls(calls, reg)

	if len(batches) != 1 {
		t.Fatalf("want 1 batch, got %d", len(batches))
	}
	if !batches[0].Concurrent {
		t.Errorf("background commands should be concurrent")
	}
}
