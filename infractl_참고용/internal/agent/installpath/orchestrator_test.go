// Package installpath
// File: orchestrator_test.go
// Description: Unit tests for installpath Decide skeleton behavior.
// Responsibility: Assert deterministic no-op decisions in PR2 skeleton.

package installpath

import (
	"testing"

	"github.com/yourorg/infractl/internal/agent/taskctx"
	"github.com/yourorg/infractl/internal/tools"
)

func TestDecideInstallSubmitJobSkeletonNoAutoInvoke(t *testing.T) {
	meta := tools.TaskProgressMetadata{
		TaskKey:              "job:42",
		JobID:                42,
		ExecutionStatus:      "submitted",
		VerificationRequired: true,
	}
	next := Decide(taskctx.KindInstall, "submit_job", tools.ToolOutcome{Success: true, MetadataJSON: meta.JSON()}, PipelineState{})
	if next.AutoInvoke {
		t.Fatalf("expected no immediate auto invoke on submit_job")
	}
}

func TestDecideJobCompleteSuggestsVerifyCompleteForInstall(t *testing.T) {
	next := Decide(
		taskctx.KindInstall,
		"job_complete",
		tools.ToolOutcome{Success: true},
		PipelineState{
			LastJobID:     42,
			PendingVerify: PendingVerify{JobID: 42, TaskKey: "job:42"},
		},
	)
	if !next.AutoInvoke {
		t.Fatalf("expected auto verify action")
	}
	if next.Tool != "verify_complete" {
		t.Fatalf("tool = %q, want verify_complete", next.Tool)
	}
	if got, _ := next.Args["task_key"].(string); got != "job:42" {
		t.Fatalf("task_key = %q, want job:42", got)
	}
}
