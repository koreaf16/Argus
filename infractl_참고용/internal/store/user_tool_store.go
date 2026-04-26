// Package store
// File: user_tool_store.go
// Description: 사용자 정의 도구 등록 정보 저장소
// Responsibility: user_tools 테이블 CRUD 및 UserToolStore 인터페이스 정의

package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

const createUserToolsSQL = `
CREATE TABLE IF NOT EXISTS user_tools (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    script_path TEXT NOT NULL,
    parameters  TEXT NOT NULL DEFAULT '{}',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);`

// UserToolEntry는 user_tools 테이블의 단일 행이다.
type UserToolEntry struct {
	ID          int64
	Name        string // 고유 도구명 (예: "cleanup_archive")
	Description string // LLM에 표시되는 설명
	ScriptPath  string // ~/.infractl/scripts/<name>.sh
	Parameters  string // JSON 파라미터 스키마
	CreatedAt   time.Time
}

// UserToolStore는 사용자 정의 도구 영속화를 담당하는 인터페이스이다.
type UserToolStore interface {
	// SaveUserTool은 사용자 도구를 저장하고 ID를 반환한다.
	SaveUserTool(ctx context.Context, entry UserToolEntry) (int64, error)

	// GetUserTool은 이름으로 사용자 도구를 조회한다.
	GetUserTool(ctx context.Context, name string) (UserToolEntry, error)

	// ListUserTools는 저장된 사용자 도구 목록을 반환한다.
	ListUserTools(ctx context.Context) ([]UserToolEntry, error)

	// DeleteUserTool은 이름으로 사용자 도구를 삭제한다.
	DeleteUserTool(ctx context.Context, name string) error
}

// initUserToolSchema는 user_tools 테이블을 생성한다.
func (s *SQLiteStore) initUserToolSchema(ctx context.Context) {
	if _, err := s.db.ExecContext(ctx, createUserToolsSQL); err != nil {
		slog.Warn("create user_tools table", "err", err)
	}
}

// SaveUserTool은 사용자 도구를 저장하고 ID를 반환한다.
// 같은 이름이 이미 있으면 업데이트한다 (UPSERT).
func (s *SQLiteStore) SaveUserTool(ctx context.Context, entry UserToolEntry) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO user_tools (name, description, script_path, parameters, created_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
		   description=excluded.description,
		   script_path=excluded.script_path,
		   parameters=excluded.parameters`,
		entry.Name, entry.Description, entry.ScriptPath,
		entry.Parameters, time.Now().UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("save user tool %s: %w", entry.Name, err)
	}
	id, _ := res.LastInsertId()
	slog.Info("user tool saved", "name", entry.Name)
	return id, nil
}

// GetUserTool은 이름으로 사용자 도구를 조회한다.
func (s *SQLiteStore) GetUserTool(ctx context.Context, name string) (UserToolEntry, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, description, script_path, parameters, created_at
		 FROM user_tools WHERE name=?`, name)
	return scanUserTool(row)
}

// ListUserTools는 저장된 모든 사용자 도구를 이름순으로 반환한다.
func (s *SQLiteStore) ListUserTools(ctx context.Context) ([]UserToolEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, description, script_path, parameters, created_at
		 FROM user_tools ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list user tools: %w", err)
	}
	defer rows.Close()

	var result []UserToolEntry
	for rows.Next() {
		entry, err := scanUserTool(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user tools: %w", err)
	}
	return result, nil
}

// DeleteUserTool은 이름으로 사용자 도구를 삭제한다.
func (s *SQLiteStore) DeleteUserTool(ctx context.Context, name string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM user_tools WHERE name=?`, name)
	if err != nil {
		return fmt.Errorf("delete user tool %s: %w", name, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user tool not found: %s", name)
	}
	slog.Info("user tool deleted", "name", name)
	return nil
}

type userToolScanner interface {
	Scan(dest ...interface{}) error
}

func scanUserTool(row userToolScanner) (UserToolEntry, error) {
	var entry UserToolEntry
	err := row.Scan(
		&entry.ID, &entry.Name, &entry.Description,
		&entry.ScriptPath, &entry.Parameters, &entry.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return UserToolEntry{}, fmt.Errorf("user tool not found")
	}
	if err != nil {
		return UserToolEntry{}, fmt.Errorf("scan user tool: %w", err)
	}
	return entry, nil
}
