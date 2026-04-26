// Package store
// File: audit_sync_engine.go
// Description: 원격 감사 DB 동기화 엔진 — push/pull 및 outbox 재시도
// Responsibility: SFTP pull-on-attach + SSH sqlite3 push-on-write + outbox 재시도 루프

package store

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// AuditRemoteTransport는 원격 서버와의 파일/명령 통신 인터페이스다.
// 순환 임포트 방지를 위해 executor 대신 이 인터페이스를 사용한다.
type AuditRemoteTransport interface {
	// DownloadFile은 원격 파일을 로컬 경로로 다운로드한다.
	DownloadFile(ctx context.Context, remotePath, localPath string) error
	// RunSQL은 원격 sqlite3 CLI로 단일 SQL 문을 실행한다.
	RunSQL(ctx context.Context, dbPath, sql string) error
}

// SyncEngine은 로컬 SQLiteStore와 원격 audit.db 사이의 동기화를 담당한다.
type SyncEngine struct {
	store *SQLiteStore
}

// NewSyncEngine은 SyncEngine을 생성한다.
func NewSyncEngine(store *SQLiteStore) *SyncEngine {
	return &SyncEngine{store: store}
}

// PullOnAttach는 원격 audit.db를 SFTP로 내려받아 로컬 DB에 병합한다.
// UUID PK 기반 INSERT OR IGNORE를 사용해 멱등성을 보장한다.
func (e *SyncEngine) PullOnAttach(ctx context.Context, transport AuditRemoteTransport, remoteAuditPath, serverName string) error {
	tmp, err := os.CreateTemp("", "infractl-audit-pull-*.db")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmp.Close()
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := transport.DownloadFile(ctx, remoteAuditPath, tmpPath); err != nil {
		return fmt.Errorf("download remote audit.db: %w", err)
	}

	if err := e.mergeFromFile(ctx, tmpPath, serverName); err != nil {
		return fmt.Errorf("merge remote audit db: %w", err)
	}
	slog.Info("audit pull complete", "server", serverName)
	return nil
}

// mergeFromFile은 ATTACH DATABASE로 임시 DB를 마운트하고 INSERT OR IGNORE로 병합한다.
func (e *SyncEngine) mergeFromFile(ctx context.Context, dbPath, serverName string) error {
	if _, err := e.store.db.ExecContext(ctx, fmt.Sprintf("ATTACH DATABASE '%s' AS remote", dbPath)); err != nil {
		return fmt.Errorf("attach database: %w", err)
	}
	defer func() {
		if _, err := e.store.db.ExecContext(ctx, "DETACH DATABASE remote"); err != nil {
			slog.Warn("audit merge: detach", "err", err)
		}
	}()

	mergeStmts := []string{
		"INSERT OR IGNORE INTO audit_runs SELECT * FROM remote.audit_runs WHERE server_name='" + serverName + "'",
		"INSERT OR IGNORE INTO audit_run_plans SELECT * FROM remote.audit_run_plans WHERE run_id IN (SELECT id FROM audit_runs WHERE server_name='" + serverName + "')",
		"INSERT OR IGNORE INTO audit_planned_steps SELECT * FROM remote.audit_planned_steps WHERE plan_id IN (SELECT id FROM audit_run_plans WHERE run_id IN (SELECT id FROM audit_runs WHERE server_name='" + serverName + "'))",
		"INSERT OR IGNORE INTO audit_steps SELECT * FROM remote.audit_steps WHERE run_id IN (SELECT id FROM audit_runs WHERE server_name='" + serverName + "')",
		"INSERT OR IGNORE INTO audit_checks SELECT * FROM remote.audit_checks WHERE server_name='" + serverName + "'",
		"INSERT OR IGNORE INTO audit_privilege_events SELECT * FROM remote.audit_privilege_events WHERE server_name='" + serverName + "'",
		"INSERT OR IGNORE INTO audit_file_snapshots SELECT * FROM remote.audit_file_snapshots WHERE privilege_event_id IN (SELECT id FROM audit_privilege_events WHERE server_name='" + serverName + "')",
	}

	for _, stmt := range mergeStmts {
		if _, err := e.store.db.ExecContext(ctx, stmt); err != nil {
			slog.Warn("audit merge stmt", "err", err, "stmt", stmt[:min(40, len(stmt))])
		}
	}
	return nil
}

// PushSQL은 SQL 문을 원격 sqlite3로 즉시 실행 시도하고, 실패 시 outbox에 적재한다.
func (e *SyncEngine) PushSQL(ctx context.Context, transport AuditRemoteTransport, serverName, remoteAuditPath, sql string) {
	if transport == nil {
		return
	}
	if err := transport.RunSQL(ctx, remoteAuditPath, sql); err != nil {
		slog.Debug("audit push failed, queuing outbox", "server", serverName, "err", err)
		if qErr := e.store.InsertSyncOutbox(ctx, serverName, sql); qErr != nil {
			slog.Warn("audit outbox enqueue", "err", qErr)
		}
		return
	}
	slog.Debug("audit push ok", "server", serverName)
}

// RetryOutbox는 serverName의 대기 중인 outbox 항목을 재시도한다.
// 성공 시 삭제, 실패 시 attempts+1 및 지수 백오프로 next_try_at 갱신.
func (e *SyncEngine) RetryOutbox(ctx context.Context, transport AuditRemoteTransport, serverName, remoteAuditPath string) {
	if transport == nil {
		return
	}
	entries, err := e.store.ListPendingSyncOutbox(ctx, serverName, 50)
	if err != nil {
		slog.Warn("audit outbox list", "err", err)
		return
	}
	for _, entry := range entries {
		if err := transport.RunSQL(ctx, remoteAuditPath, entry.PayloadSQL); err != nil {
			backoff := time.Duration(1<<min(entry.Attempts, 6)) * time.Minute
			if fErr := e.store.FailSyncOutbox(ctx, entry.ID, err.Error(), time.Now().Add(backoff)); fErr != nil {
				slog.Warn("audit outbox fail update", "err", fErr)
			}
			continue
		}
		if dErr := e.store.DeleteSyncOutbox(ctx, entry.ID); dErr != nil {
			slog.Warn("audit outbox delete", "err", dErr)
		}
	}
}

// QuoteSQL은 문자열 값을 SQLite 리터럴로 안전하게 이스케이프한다.
// 작은따옴표를 두 개로 치환한다.
func QuoteSQL(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

