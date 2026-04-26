// Package store
// File: audit_check_store.go
// Description: audit_checks 테이블 CRUD 및 스키마 초기화
// Responsibility: 시스템 점검 결과 스냅샷 영속화

package store

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const createAuditChecksSQL = `
CREATE TABLE IF NOT EXISTS audit_checks (
    id           TEXT PRIMARY KEY,
    run_id       TEXT DEFAULT NULL,
    workspace_id TEXT NOT NULL DEFAULT '',
    server_name  TEXT NOT NULL DEFAULT 'localhost',
    category     TEXT NOT NULL DEFAULT 'custom',
    name         TEXT NOT NULL DEFAULT '',
    verdict      TEXT NOT NULL DEFAULT 'info',
    raw_output   TEXT NOT NULL DEFAULT '',
    findings_md  TEXT NOT NULL DEFAULT '',
    checked_at   DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_checks_server  ON audit_checks(server_name, checked_at);
CREATE INDEX IF NOT EXISTS idx_audit_checks_verdict ON audit_checks(verdict);
`

// AuditCheckStore는 audit_checks 테이블 접근 인터페이스이다.
type AuditCheckStore interface {
	InsertAuditCheck(ctx context.Context, check AuditCheck) error
	ListAuditChecks(ctx context.Context, serverName string, since time.Time, limit int) ([]AuditCheck, error)
}

// initAuditCheckSchema는 audit_checks 테이블과 인덱스를 생성한다.
func (s *SQLiteStore) initAuditCheckSchema(ctx context.Context) {
	if _, err := s.db.ExecContext(ctx, createAuditChecksSQL); err != nil {
		slog.Warn("init audit_checks schema", "err", err)
	}
}

// InsertAuditCheck는 점검 결과를 삽입한다 (멱등).
func (s *SQLiteStore) InsertAuditCheck(ctx context.Context, check AuditCheck) error {
	var runID interface{}
	if check.RunID != "" {
		runID = check.RunID
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO audit_checks
		 (id, run_id, workspace_id, server_name, category, name, verdict, raw_output, findings_md, checked_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		check.ID, runID, check.ServerID, check.ServerName,
		check.Category, check.Name, string(check.Verdict),
		truncate(check.RawOutput, 16*1024), check.FindingsMD,
		check.CheckedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert audit check: %w", err)
	}
	return nil
}

// ListAuditChecks는 server_name 기준으로 최신순 점검 결과를 반환한다.
func (s *SQLiteStore) ListAuditChecks(ctx context.Context, serverName string, since time.Time, limit int) ([]AuditCheck, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, COALESCE(run_id,''), workspace_id, server_name, category, name, verdict, raw_output, findings_md, checked_at
		 FROM audit_checks
		 WHERE server_name=? AND checked_at >= ?
		 ORDER BY checked_at DESC LIMIT ?`,
		serverName, since.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("list audit checks: %w", err)
	}
	defer rows.Close()
	var checks []AuditCheck
	for rows.Next() {
		var c AuditCheck
		var verdict string
		if err := rows.Scan(&c.ID, &c.RunID, &c.ServerID, &c.ServerName,
			&c.Category, &c.Name, &verdict, &c.RawOutput, &c.FindingsMD, &c.CheckedAt); err != nil {
			return nil, fmt.Errorf("scan audit check: %w", err)
		}
		c.Verdict = AuditCheckVerdict(verdict)
		checks = append(checks, c)
	}
	return checks, rows.Err()
}

func truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	return s[:maxBytes] + "\n...[truncated]"
}
