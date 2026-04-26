// Package store
// File: audit_run_store.go
// Description: audit_runs 테이블 CRUD 및 스키마 초기화
// Responsibility: 작업 세션(Run) 헤더의 생성·조회·상태 업데이트

package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

const createAuditRunsSQL = `
CREATE TABLE IF NOT EXISTS audit_runs (
    id            TEXT PRIMARY KEY,
    workspace_id  TEXT NOT NULL DEFAULT '',
    server_name   TEXT NOT NULL DEFAULT 'localhost',
    kind          TEXT NOT NULL DEFAULT 'ad-hoc',
    title         TEXT NOT NULL DEFAULT '',
    user_prompt   TEXT NOT NULL DEFAULT '',
    active_plan_id TEXT DEFAULT NULL,
    status        TEXT NOT NULL DEFAULT 'planning',
    started_at    DATETIME NOT NULL,
    ended_at      DATETIME DEFAULT NULL,
    summary_md    TEXT NOT NULL DEFAULT '',
    metadata_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_audit_runs_server  ON audit_runs(server_name, status);
CREATE INDEX IF NOT EXISTS idx_audit_runs_started ON audit_runs(started_at);
`

// AuditRunStore는 audit_runs 테이블 접근 인터페이스이다.
type AuditRunStore interface {
	InsertAuditRun(ctx context.Context, run AuditRun) error
	UpdateAuditRunStatus(ctx context.Context, id string, status AuditRunStatus, endedAt *time.Time, summaryMD string) error
	UpdateAuditRunTitle(ctx context.Context, id, title, userPrompt string) error
	UpdateAuditRunActivePlan(ctx context.Context, id, planID string) error
	GetAuditRun(ctx context.Context, id string) (AuditRun, error)
}

// initAuditRunSchema는 audit_runs 테이블과 인덱스를 생성한다.
func (s *SQLiteStore) initAuditRunSchema(ctx context.Context) {
	if _, err := s.db.ExecContext(ctx, createAuditRunsSQL); err != nil {
		slog.Warn("init audit_runs schema", "err", err)
	}
}

// InsertAuditRun은 새 run 을 삽입한다. UUID PK 충돌 시 무시(멱등).
func (s *SQLiteStore) InsertAuditRun(ctx context.Context, run AuditRun) error {
	meta := run.MetadataJSON
	if meta == "" {
		meta = "{}"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO audit_runs
		 (id, workspace_id, server_name, kind, title, user_prompt, active_plan_id, status, started_at, ended_at, summary_md, metadata_json)
		 VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?)`,
		run.ID, run.ServerID, run.ServerName,
		string(run.Kind), run.Title, run.UserPrompt,
		run.ActivePlanID, string(run.Status),
		run.StartedAt.UTC(), nullTime(run.EndedAt),
		run.SummaryMD, meta,
	)
	if err != nil {
		return fmt.Errorf("insert audit run: %w", err)
	}
	return nil
}

// UpdateAuditRunStatus는 run 의 상태·종료 시각·요약을 갱신한다.
func (s *SQLiteStore) UpdateAuditRunStatus(ctx context.Context, id string, status AuditRunStatus, endedAt *time.Time, summaryMD string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE audit_runs SET status=?, ended_at=?, summary_md=? WHERE id=?`,
		string(status), nullTime(endedAt), summaryMD, id,
	)
	if err != nil {
		return fmt.Errorf("update audit run status %s: %w", id, err)
	}
	return nil
}

// UpdateAuditRunTitle은 run 의 제목과 사용자 프롬프트를 갱신한다.
func (s *SQLiteStore) UpdateAuditRunTitle(ctx context.Context, id, title, userPrompt string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE audit_runs SET title=?, user_prompt=? WHERE id=?`,
		title, userPrompt, id,
	)
	if err != nil {
		return fmt.Errorf("update audit run title %s: %w", id, err)
	}
	return nil
}

// UpdateAuditRunActivePlan은 run 의 active_plan_id 와 status=running 으로 전환한다.
func (s *SQLiteStore) UpdateAuditRunActivePlan(ctx context.Context, id, planID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE audit_runs SET active_plan_id=?, status='running' WHERE id=?`,
		planID, id,
	)
	if err != nil {
		return fmt.Errorf("update audit run plan %s: %w", id, err)
	}
	return nil
}

// GetAuditRun은 ID로 단일 run 을 조회한다.
func (s *SQLiteStore) GetAuditRun(ctx context.Context, id string) (AuditRun, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, server_name, kind, title, user_prompt,
		        COALESCE(active_plan_id,''), status, started_at, ended_at, summary_md, metadata_json
		 FROM audit_runs WHERE id=?`, id)
	run, err := scanAuditRun(row)
	if err == nil {
		return run, nil
	}
	// prefix 조회 fallback (8+ chars prefix)
	if len(id) >= 8 {
		row2 := s.db.QueryRowContext(ctx,
			`SELECT id, workspace_id, server_name, kind, title, user_prompt,
			        COALESCE(active_plan_id,''), status, started_at, ended_at, summary_md, metadata_json
			 FROM audit_runs WHERE id LIKE ? LIMIT 1`, id+"%")
		run2, err2 := scanAuditRun(row2)
		if err2 == nil {
			return run2, nil
		}
	}
	return AuditRun{}, err
}

type auditRunScanner interface {
	Scan(dest ...interface{}) error
}

func scanAuditRun(row auditRunScanner) (AuditRun, error) {
	var r AuditRun
	var kind, status string
	var endedAt sql.NullTime
	err := row.Scan(
		&r.ID, &r.ServerID, &r.ServerName,
		&kind, &r.Title, &r.UserPrompt,
		&r.ActivePlanID, &status,
		&r.StartedAt, &endedAt,
		&r.SummaryMD, &r.MetadataJSON,
	)
	if err != nil {
		return AuditRun{}, fmt.Errorf("scan audit run: %w", err)
	}
	r.Kind = AuditRunKind(kind)
	r.Status = AuditRunStatus(status)
	if endedAt.Valid {
		r.EndedAt = &endedAt.Time
	}
	return r, nil
}

func nullTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.UTC()
}
