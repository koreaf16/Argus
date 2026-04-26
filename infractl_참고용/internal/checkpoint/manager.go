// Package checkpoint
// File: manager.go
// Description: 체크포인트 생성/조회/롤백 로직 관리자
// Responsibility: 체크포인트 CRUD와 롤백 명령 추출 제공

package checkpoint

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/yourorg/infractl/internal/store"
)

// Manager는 체크포인트 생명주기를 관리한다.
type Manager struct {
	store store.CheckpointStore
}

// NewManager는 Manager를 생성한다.
func NewManager(s store.CheckpointStore) *Manager {
	return &Manager{store: s}
}

// Create는 새 체크포인트를 생성하고 ID를 반환한다.
func (m *Manager) Create(ctx context.Context, server, description, toolName string, snap store.CheckpointSnapshot) (int64, error) {
	id, err := m.store.SaveCheckpoint(ctx, store.Checkpoint{
		Server:      server,
		Description: description,
		ToolName:    toolName,
		Snapshot:    snap,
	})
	if err != nil {
		return 0, fmt.Errorf("create checkpoint: %w", err)
	}
	slog.Info("checkpoint created", "id", id, "server", server, "desc", description)
	return id, nil
}

// GetLatest는 서버의 가장 최근 체크포인트를 반환한다.
func (m *Manager) GetLatest(ctx context.Context, server string) (store.Checkpoint, error) {
	return m.store.GetLatestCheckpoint(ctx, server)
}

// Get은 ID로 체크포인트를 반환한다.
func (m *Manager) Get(ctx context.Context, id int64) (store.Checkpoint, error) {
	return m.store.GetCheckpoint(ctx, id)
}

// List는 서버의 체크포인트 목록을 반환한다.
func (m *Manager) List(ctx context.Context, server string, limit int) ([]store.Checkpoint, error) {
	return m.store.ListCheckpoints(ctx, server, limit)
}

// BuildRollbackCommand는 체크포인트에서 롤백 명령을 추출한다.
func (m *Manager) BuildRollbackCommand(cp store.Checkpoint) string {
	return cp.Snapshot.RollbackCommand
}

// CreateMandatory는 모든 mutation 도구에 무조건 체크포인트를 생성한다.
// args에 rollback_command/checkpoint_description이 없어도 생성한다.
func (m *Manager) CreateMandatory(ctx context.Context, serverName, toolName string, args map[string]interface{}) error {
	desc, _ := args["checkpoint_description"].(string)
	if desc == "" {
		desc = fmt.Sprintf("%s 실행 전 자동 체크포인트", toolName)
	}
	rollback, _ := args["rollback_command"].(string)
	snap := store.CheckpointSnapshot{RollbackCommand: rollback}
	if cmd, ok := args["command"].(string); ok {
		snap.CommandUsed = cmd
	} else if sql, ok := args["sql"].(string); ok {
		snap.CommandUsed = sql
	}
	if _, err := m.Create(ctx, serverName, desc, toolName, snap); err != nil {
		slog.Warn("mandatory checkpoint failed", "tool", toolName, "err", err)
	}
	return nil
}

// CreateFromArgs는 도구 호출 인자에서 체크포인트를 자동 생성한다.
// args에 "checkpoint_description" 또는 "rollback_command"가 있을 때만 생성한다.
func (m *Manager) CreateFromArgs(ctx context.Context, server, toolName string, args map[string]interface{}) {
	desc, _ := args["checkpoint_description"].(string)
	rollback, _ := args["rollback_command"].(string)
	if desc == "" && rollback == "" {
		return
	}
	if desc == "" {
		desc = fmt.Sprintf("%s 실행 전 자동 체크포인트", toolName)
	}
	snap := store.CheckpointSnapshot{RollbackCommand: rollback}
	if cmd, ok := args["command"].(string); ok {
		snap.CommandUsed = cmd
	}
	if _, err := m.Create(ctx, server, desc, toolName, snap); err != nil {
		slog.Warn("auto checkpoint failed", "err", err)
	}
}
