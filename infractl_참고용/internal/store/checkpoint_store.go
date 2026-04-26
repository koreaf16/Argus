// Package store
// File: checkpoint_store.go
// Description: 체크포인트 타입 정의, 저장소 인터페이스, SQLite 구현
// Responsibility: checkpoints 테이블 CRUD 및 최신/목록 조회 제공

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// CheckpointSnapshot은 체크포인트 시점의 상태 스냅샷이다.
type CheckpointSnapshot struct {
	Parameter       string `json:"parameter,omitempty"`
	OldValue        string `json:"old_value,omitempty"`
	NewValue        string `json:"new_value,omitempty"`
	CommandUsed     string `json:"command_used,omitempty"`
	RollbackCommand string `json:"rollback_command,omitempty"`
}

// Checkpoint는 특정 시점의 서버 상태 스냅샷이다.
type Checkpoint struct {
	ID          int64
	Server      string
	Description string
	ToolName    string
	Snapshot    CheckpointSnapshot
	CreatedAt   time.Time
}

// CheckpointStore는 체크포인트 영속화 인터페이스이다.
type CheckpointStore interface {
	SaveCheckpoint(ctx context.Context, cp Checkpoint) (int64, error)
	GetCheckpoint(ctx context.Context, id int64) (Checkpoint, error)
	GetLatestCheckpoint(ctx context.Context, server string) (Checkpoint, error)
	ListCheckpoints(ctx context.Context, server string, limit int) ([]Checkpoint, error)
}

const createCheckpointSQL = `
CREATE TABLE IF NOT EXISTS checkpoints (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    server      TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    tool_name   TEXT    NOT NULL DEFAULT '',
    snapshot    TEXT    NOT NULL DEFAULT '{}',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_checkpoints_server ON checkpoints(server, created_at);
`

// initCheckpointSchema는 checkpoints 테이블을 생성한다.
func (s *SQLiteStore) initCheckpointSchema(ctx context.Context) {
	if _, err := s.db.ExecContext(ctx, createCheckpointSQL); err != nil {
		slog.Warn("init checkpoint schema", "err", err)
	}
}

// SaveCheckpoint는 체크포인트를 저장하고 ID를 반환한다.
func (s *SQLiteStore) SaveCheckpoint(ctx context.Context, cp Checkpoint) (int64, error) {
	snapJSON, err := json.Marshal(cp.Snapshot)
	if err != nil {
		return 0, fmt.Errorf("marshal snapshot: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO checkpoints (server, description, tool_name, snapshot, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		cp.Server, cp.Description, cp.ToolName, string(snapJSON), time.Now(),
	)
	if err != nil {
		return 0, fmt.Errorf("save checkpoint: %w", err)
	}
	return res.LastInsertId()
}

// GetCheckpoint는 ID로 체크포인트를 조회한다.
func (s *SQLiteStore) GetCheckpoint(ctx context.Context, id int64) (Checkpoint, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, server, description, tool_name, snapshot, created_at FROM checkpoints WHERE id = ?`, id)
	return scanCheckpointRow(row.Scan)
}

// GetLatestCheckpoint는 서버의 가장 최근 체크포인트를 반환한다.
func (s *SQLiteStore) GetLatestCheckpoint(ctx context.Context, server string) (Checkpoint, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, server, description, tool_name, snapshot, created_at
		 FROM checkpoints WHERE server = ? ORDER BY created_at DESC LIMIT 1`, server)
	return scanCheckpointRow(row.Scan)
}

// ListCheckpoints는 서버의 체크포인트 목록을 최신순으로 반환한다. server=""이면 전체.
func (s *SQLiteStore) ListCheckpoints(ctx context.Context, server string, limit int) ([]Checkpoint, error) {
	var rows *sql.Rows
	var err error
	if server == "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, server, description, tool_name, snapshot, created_at
			 FROM checkpoints ORDER BY created_at DESC LIMIT ?`, limit)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, server, description, tool_name, snapshot, created_at
			 FROM checkpoints WHERE server = ? ORDER BY created_at DESC LIMIT ?`, server, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list checkpoints: %w", err)
	}
	defer rows.Close()

	var result []Checkpoint
	for rows.Next() {
		cp, err := scanCheckpointRow(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan checkpoint: %w", err)
		}
		result = append(result, cp)
	}
	return result, nil
}

// scanCheckpointRow는 Scan 함수로 Checkpoint를 구성한다.
func scanCheckpointRow(scan func(dest ...interface{}) error) (Checkpoint, error) {
	var cp Checkpoint
	var snapJSON string
	if err := scan(&cp.ID, &cp.Server, &cp.Description, &cp.ToolName, &snapJSON, &cp.CreatedAt); err != nil {
		return Checkpoint{}, fmt.Errorf("scan checkpoint: %w", err)
	}
	if err := json.Unmarshal([]byte(snapJSON), &cp.Snapshot); err != nil {
		return Checkpoint{}, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	return cp, nil
}
