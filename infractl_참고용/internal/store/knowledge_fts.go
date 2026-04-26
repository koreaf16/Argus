// Package store
// File: knowledge_fts.go
// Description: knowledge_base 테이블 및 FTS5 가상 테이블 스키마 초기화 및 마이그레이션
// Responsibility: Phase 6 에러 패턴 학습 / 지식 베이스의 DDL 및 기존 DB 마이그레이션

package store

import (
	"context"
	"log/slog"
)

const createKnowledgeBaseSQL = `
CREATE TABLE IF NOT EXISTS knowledge_base (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    category            TEXT NOT NULL DEFAULT 'tip',
    title               TEXT NOT NULL DEFAULT '',
    situation           TEXT NOT NULL DEFAULT '',
    resolution          TEXT NOT NULL DEFAULT '',
    tool_name           TEXT NOT NULL DEFAULT '',
    error_pattern       TEXT NOT NULL DEFAULT '',
    success_command     TEXT NOT NULL DEFAULT '',
    server_name         TEXT NOT NULL DEFAULT '',
    task_key            TEXT NOT NULL DEFAULT '',
    command_key         TEXT NOT NULL DEFAULT '',
    workflow_json       TEXT NOT NULL DEFAULT '[]',
    embedding           BLOB,
    source_execution_id INTEGER REFERENCES execution_logs(id),
    confidence          REAL NOT NULL DEFAULT 1.0,
    use_count           INTEGER NOT NULL DEFAULT 0,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at        DATETIME
);`

const createKnowledgeFTSSQL = `
CREATE VIRTUAL TABLE IF NOT EXISTS knowledge_fts USING fts5(
    title, situation, resolution, error_pattern, category,
    content='knowledge_base', content_rowid='id'
);`

const createKnowledgeIdxSQL = `
CREATE INDEX IF NOT EXISTS idx_kb_category ON knowledge_base(category);
CREATE INDEX IF NOT EXISTS idx_kb_tool ON knowledge_base(tool_name);
CREATE INDEX IF NOT EXISTS idx_kb_error ON knowledge_base(error_pattern);
CREATE INDEX IF NOT EXISTS idx_kb_task_scope ON knowledge_base(category, server_name, task_key);
CREATE INDEX IF NOT EXISTS idx_kb_command_key ON knowledge_base(category, command_key);`

// initKnowledgeFTSSchema는 knowledge_base 테이블과 FTS5 가상 테이블을 생성하고 마이그레이션한다.
// Phase 5 기존 DB(6컬럼)가 있으면 컬럼을 추가하고 FTS5를 재생성한다.
func (s *SQLiteStore) initKnowledgeFTSSchema(ctx context.Context) {
	// 기존 테이블 생성 (없으면 생성)
	if _, err := s.db.ExecContext(ctx, createKnowledgeBaseSQL); err != nil {
		slog.Warn("create knowledge_base table", "err", err)
	}

	// Phase 5 → Phase 6 마이그레이션: 컬럼 추가 (이미 있으면 무시)
	alterStmts := []string{
		`ALTER TABLE knowledge_base ADD COLUMN category TEXT NOT NULL DEFAULT 'tip'`,
		`ALTER TABLE knowledge_base ADD COLUMN tool_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_base ADD COLUMN success_command TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_base ADD COLUMN embedding BLOB`,
		`ALTER TABLE knowledge_base ADD COLUMN source_execution_id INTEGER REFERENCES execution_logs(id)`,
		`ALTER TABLE knowledge_base ADD COLUMN confidence REAL NOT NULL DEFAULT 1.0`,
		`ALTER TABLE knowledge_base ADD COLUMN use_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE knowledge_base ADD COLUMN last_used_at DATETIME`,
		`ALTER TABLE knowledge_base ADD COLUMN server_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_base ADD COLUMN task_key TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_base ADD COLUMN command_key TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE knowledge_base ADD COLUMN workflow_json TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE knowledge_base ADD COLUMN updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP`,
	}
	for _, stmt := range alterStmts {
		// ALTER TABLE 실패는 컬럼이 이미 존재하는 경우이므로 무시한다.
		s.db.ExecContext(ctx, stmt) //nolint: errcheck
	}

	// FTS5 가상 테이블: category 컬럼 추가로 재생성이 필요하면 DROP 후 재생성
	// content 테이블이므로 실제 데이터는 knowledge_base에 있어 DROP해도 안전하다.
	if _, err := s.db.ExecContext(ctx, createKnowledgeFTSSQL); err != nil {
		slog.Warn("create knowledge_fts virtual table", "err", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO knowledge_fts(knowledge_fts) VALUES ('rebuild')`); err != nil {
		slog.Debug("rebuild knowledge_fts", "err", err)
	}

	// 인덱스 생성
	idxStmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_kb_category ON knowledge_base(category)`,
		`CREATE INDEX IF NOT EXISTS idx_kb_tool ON knowledge_base(tool_name)`,
		`CREATE INDEX IF NOT EXISTS idx_kb_error ON knowledge_base(error_pattern)`,
		`CREATE INDEX IF NOT EXISTS idx_kb_task_scope ON knowledge_base(category, server_name, task_key)`,
		`CREATE INDEX IF NOT EXISTS idx_kb_command_key ON knowledge_base(category, command_key)`,
	}
	for _, stmt := range idxStmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			slog.Warn("create knowledge_base index", "err", err)
		}
	}
}
