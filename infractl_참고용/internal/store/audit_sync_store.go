// Package store
// File: audit_sync_store.go
// Description: sync_outbox 테이블 — 원격 감사 DB 동기화 재시도 큐
// Responsibility: 원격 push 실패 시 재시도 큐 영속화 및 CRUD

package store

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const createAuditSyncOutboxSQL = `
CREATE TABLE IF NOT EXISTS audit_sync_outbox (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    target_server TEXT NOT NULL,
    payload_sql  TEXT NOT NULL,
    attempts     INTEGER NOT NULL DEFAULT 0,
    last_error   TEXT NOT NULL DEFAULT '',
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    next_try_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_audit_outbox_target ON audit_sync_outbox(target_server, next_try_at);
`

// AuditSyncOutboxEntry는 sync_outbox 테이블의 단일 행이다.
type AuditSyncOutboxEntry struct {
	ID           int64
	TargetServer string
	PayloadSQL   string
	Attempts     int
	LastError    string
	CreatedAt    time.Time
	NextTryAt    time.Time
}

// AuditSyncStore는 sync_outbox 테이블 접근 인터페이스이다.
type AuditSyncStore interface {
	InsertSyncOutbox(ctx context.Context, server, payloadSQL string) error
	ListPendingSyncOutbox(ctx context.Context, server string, limit int) ([]AuditSyncOutboxEntry, error)
	DeleteSyncOutbox(ctx context.Context, id int64) error
	FailSyncOutbox(ctx context.Context, id int64, errMsg string, nextTry time.Time) error
}

// initAuditSyncOutboxSchema는 sync_outbox 테이블과 인덱스를 생성한다.
func (s *SQLiteStore) initAuditSyncOutboxSchema(ctx context.Context) {
	if _, err := s.db.ExecContext(ctx, createAuditSyncOutboxSQL); err != nil {
		slog.Warn("init audit_sync_outbox schema", "err", err)
	}
}

// InsertSyncOutbox는 미전송 SQL 페이로드를 큐에 추가한다.
func (s *SQLiteStore) InsertSyncOutbox(ctx context.Context, server, payloadSQL string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_sync_outbox (target_server, payload_sql, created_at, next_try_at) VALUES (?, ?, ?, ?)`,
		server, payloadSQL, time.Now().UTC(), time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert sync outbox: %w", err)
	}
	return nil
}

// ListPendingSyncOutbox는 전송 준비된 항목을 오래된 순으로 반환한다.
func (s *SQLiteStore) ListPendingSyncOutbox(ctx context.Context, server string, limit int) ([]AuditSyncOutboxEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, target_server, payload_sql, attempts, last_error, created_at, next_try_at
		 FROM audit_sync_outbox
		 WHERE target_server=? AND next_try_at <= ?
		 ORDER BY next_try_at ASC LIMIT ?`,
		server, time.Now().UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("list sync outbox: %w", err)
	}
	defer rows.Close()
	var entries []AuditSyncOutboxEntry
	for rows.Next() {
		var e AuditSyncOutboxEntry
		if err := rows.Scan(&e.ID, &e.TargetServer, &e.PayloadSQL, &e.Attempts, &e.LastError, &e.CreatedAt, &e.NextTryAt); err != nil {
			return nil, fmt.Errorf("scan sync outbox: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// DeleteSyncOutbox는 성공한 항목을 삭제한다.
func (s *SQLiteStore) DeleteSyncOutbox(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM audit_sync_outbox WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete sync outbox: %w", err)
	}
	return nil
}

// FailSyncOutbox는 실패한 항목의 재시도 정보를 업데이트한다.
func (s *SQLiteStore) FailSyncOutbox(ctx context.Context, id int64, errMsg string, nextTry time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE audit_sync_outbox SET attempts=attempts+1, last_error=?, next_try_at=? WHERE id=?`,
		errMsg, nextTry.UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("fail sync outbox: %w", err)
	}
	return nil
}
