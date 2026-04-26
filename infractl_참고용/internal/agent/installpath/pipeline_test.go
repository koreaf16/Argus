// Package installpath
// File: pipeline_test.go
// Description: Stateful pipeline tests for auto verify enqueue behavior.
// Responsibility: Validate submit_job -> job_complete transition queueing.

package installpath

import (
	"context"
	"testing"

	"github.com/yourorg/infractl/internal/agent/taskctx"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/tools"
)

func TestPipelineEnqueuesAutoVerifyAfterJobCompletion(t *testing.T) {
	p := New(func() taskctx.TaskKind { return taskctx.KindInstall })
	meta := tools.TaskProgressMetadata{
		TaskKey:              "job:7",
		JobID:                7,
		ExecutionStatus:      "submitted",
		VerificationRequired: true,
	}
	p.OnToolEnd(context.Background(), llm.ToolCall{
		ID:   "tc-submit",
		Type: "function",
		Function: llm.FunctionCall{
			Name:      "submit_job",
			Arguments: `{"command":"apt-get install -y nginx"}`,
		},
	}, tools.ToolOutcome{Success: true, MetadataJSON: meta.JSON()})

	p.OnJobComplete(7, true)
	action, ok := p.Dequeue()
	if !ok {
		t.Fatal("expected queued auto verify action")
	}
	if action.ToolCall.Function.Name != "verify_complete" {
		t.Fatalf("tool = %q, want verify_complete", action.ToolCall.Function.Name)
	}
}
