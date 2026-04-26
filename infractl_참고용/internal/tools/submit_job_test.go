// Package tools
// File: submit_job_test.go
// Description: submit_job metadata regression tests.
// Responsibility: Ensure JobID metadata is emitted for install-path chaining.

package tools

import (
	"context"
	"testing"
	"time"

	"github.com/yourorg/infractl/internal/background"
	"github.com/yourorg/infractl/internal/executor"
)

type submitJobExecStub struct{}

func (submitJobExecStub) Execute(context.Context, string) (executor.ExecResult, error) {
	return executor.ExecResult{Stdout: "ok", ExitCode: 0}, nil
}
func (submitJobExecStub) Target() string { return "localhost" }
func (submitJobExecStub) Host() string   { return "localhost" }

func TestSubmitJobEmitsTaskProgressMetadataWithJobID(t *testing.T) {
	mgr := background.NewManager(t.TempDir(), 1<<20)
	tool := &SubmitJobTool{Manager: mgr}

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"command":     "echo hello",
		"description": "test job",
	}, submitJobExecStub{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !out.Success {
		t.Fatalf("expected success, got: %s", out.Content)
	}

	meta, ok := ParseTaskProgressMetadata(out.MetadataJSON)
	if !ok {
		t.Fatalf("expected metadata JSON, got %q", out.MetadataJSON)
	}
	if meta.JobID <= 0 {
		t.Fatalf("expected JobID > 0, got %d", meta.JobID)
	}
	if meta.TaskKey == "" {
		t.Fatal("expected task key for job metadata")
	}
	if !meta.VerificationRequired {
		t.Fatal("expected verification_required=true")
	}
	if meta.ExecutionStatus != "submitted" {
		t.Fatalf("execution status = %q, want submitted", meta.ExecutionStatus)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, getErr := mgr.Get(meta.JobID)
		if getErr != nil {
			t.Fatalf("get job: %v", getErr)
		}
		if job.Status != background.StatusRunning {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("background job %d did not finish before timeout", meta.JobID)
}
