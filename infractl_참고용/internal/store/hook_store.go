// Package store
// File: hook_store.go
// Description: Hook 타입 정의, 저장소 인터페이스, SQLite 구현
// Responsibility: hooks 테이블 CRUD 및 이벤트별 조회 제공

package store

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Hook은 라이프사이클 이벤트에 연결된 스크립트 정의이다.
type Hook struct {
	ID         int64
	Event      string // before_execute | after_execute | on_error | on_connect | on_alert | on_session_start
	Condition  string // 매칭 조건 (빈 문자열이면 항상 실행)
	ScriptPath string // ~/.infractl/hooks/ 아래 스크립트 경로
	Enabled    bool
	CreatedAt  time.Time
}

// HookStore는 Hook 영속화 인터페이스이다.
type HookStore interface {
	SaveHook(ctx context.Context, h Hook) (int64, error)
	ListHooks(ctx context.Context) ([]Hook, error)
	ListHooksByEvent(ctx context.Context, event string) ([]Hook, error)
	SetHookEnabled(ctx context.Context, id int64, enabled bool) error
	DeleteHook(ctx context.Context, id int64) error
}

const createHookSQL = `
CREATE TABLE IF NOT EXISTS hooks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    event       TEXT    NOT NULL,
    condition   TEXT    NOT NULL DEFAULT '',
    script_path TEXT    NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 1,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_hooks_event ON hooks(event, enabled);
`

// initHookSchema는 hooks 테이블을 생성한다.
func (s *SQLiteStore) initHookSchema(ctx context.Context) {
	if _, err := s.db.ExecContext(ctx, createHookSQL); err != nil {
		slog.Warn("init hook schema", "err", err)
	}
}

// SaveHook은 Hook을 저장하고 ID를 반환한다.
func (s *SQLiteStore) SaveHook(ctx context.Context, h Hook) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO hooks (event, condition, script_path, enabled, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		h.Event, h.Condition, h.ScriptPath, boolToInt(h.Enabled), time.Now(),
	)
	if err != nil {
		return 0, fmt.Errorf("save hook: %w", err)
	}
	return res.LastInsertId()
}

// ListHooks는 모든 Hook을 반환한다.
func (s *SQLiteStore) ListHooks(ctx context.Context) ([]Hook, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, event, condition, script_path, enabled, created_at FROM hooks ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list hooks: %w", err)
	}
	defer rows.Close()
	return scanHooks(rows)
}

// ListHooksByEvent는 특정 이벤트의 활성화된 Hook 목록을 반환한다.
func (s *SQLiteStore) ListHooksByEvent(ctx context.Context, event string) ([]Hook, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, event, condition, script_path, enabled, created_at
		 FROM hooks WHERE event = ? AND enabled = 1 ORDER BY id`, event)
	if err != nil {
		return nil, fmt.Errorf("list hooks by event: %w", err)
	}
	defer rows.Close()
	return scanHooks(rows)
}

// SetHookEnabled는 Hook의 활성/비활성 상태를 변경한다.
func (s *SQLiteStore) SetHookEnabled(ctx context.Context, id int64, enabled bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE hooks SET enabled = ? WHERE id = ?`, boolToInt(enabled), id)
	if err != nil {
		return fmt.Errorf("set hook enabled: %w", err)
	}
	return nil
}

// DeleteHook은 ID로 Hook을 삭제한다.
func (s *SQLiteStore) DeleteHook(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM hooks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete hook: %w", err)
	}
	return nil
}

func scanHooks(rows interface {
	Next() bool
	Scan(dest ...interface{}) error
}) ([]Hook, error) {
	var result []Hook
	for rows.Next() {
		var h Hook
		var enabled int
		if err := rows.Scan(&h.ID, &h.Event, &h.Condition, &h.ScriptPath, &enabled, &h.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan hook: %w", err)
		}
		h.Enabled = enabled == 1
		result = append(result, h)
	}
	return result, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
