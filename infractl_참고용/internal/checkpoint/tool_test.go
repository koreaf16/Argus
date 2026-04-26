// Package checkpoint
// File: tool_test.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package checkpoint

import (
	"context"
	"testing"
	"time"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/store"
)

type checkpointStoreStub struct {
	checkpoint store.Checkpoint
}

func (s *checkpointStoreStub) SaveCheckpoint(context.Context, store.Checkpoint) (int64, error) {
	return 0, nil
}

func (s *checkpointStoreStub) GetCheckpoint(context.Context, int64) (store.Checkpoint, error) {
	return s.checkpoint, nil
}

func (s *checkpointStoreStub) GetLatestCheckpoint(context.Context, string) (store.Checkpoint, error) {
	return s.checkpoint, nil
}

func (s *checkpointStoreStub) ListCheckpoints(context.Context, string, int) ([]store.Checkpoint, error) {
	return []store.Checkpoint{s.checkpoint}, nil
}

type rollbackExecStub struct {
	target  string
	command string
}

func (e *rollbackExecStub) Execute(_ context.Context, command string) (executor.ExecResult, error) {
	e.command = command
	return executor.ExecResult{ExitCode: 0}, nil
}

func (e *rollbackExecStub) Target() string {
	return e.target
}

func (e *rollbackExecStub) Host() string {
	return e.target
}

func TestRollbackToolRejectsMismatchedExecutorTarget(t *testing.T) {
	manager := NewManager(&checkpointStoreStub{
		checkpoint: store.Checkpoint{
			ID:          7,
			Server:      "db-prod",
			Description: "before config edit",
			CreatedAt:   time.Now(),
			Snapshot: store.CheckpointSnapshot{
				RollbackCommand: "restore-command",
			},
		},
	})
	tool := &RollbackTool{Manager: manager}
	exec := &rollbackExecStub{target: "app-prod"}

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"checkpoint_id": float64(7),
	}, exec)
	if err == nil {
		t.Fatalf("expected target mismatch error")
	}
}

func TestRollbackToolExecutesOnlyOnCheckpointOwnerServer(t *testing.T) {
	manager := NewManager(&checkpointStoreStub{
		checkpoint: store.Checkpoint{
			ID:          8,
			Server:      "db-prod",
			Description: "before config edit",
			CreatedAt:   time.Now(),
			Snapshot: store.CheckpointSnapshot{
				RollbackCommand: "restore-command",
			},
		},
	})
	tool := &RollbackTool{Manager: manager}
	exec := &rollbackExecStub{target: "db-prod"}

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"checkpoint_id": float64(8),
	}, exec)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if exec.command != "restore-command" {
		t.Fatalf("expected rollback command to execute, got %q", exec.command)
	}
	if out.Content == "" {
		t.Fatalf("expected success output")
	}
}

