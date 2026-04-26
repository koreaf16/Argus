// Package store
// File: audit_privilege_store.go
// Description: audit_privilege_events / audit_file_snapshots 테이블 CRUD 및 스키마 초기화
// Responsibility: root/sudo 권한 상승 이벤트와 파일 변경 스냅샷 영속화

package store

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"time"
)

const createAuditPrivilegeEventsSQL = `
CREATE TABLE IF NOT EXISTS audit_privilege_events (
    id            TEXT PRIMARY KEY,
    run_id        TEXT DEFAULT NULL,
    workspace_id  TEXT NOT NULL DEFAULT '',
    server_name   TEXT NOT NULL DEFAULT 'localhost',
    command       TEXT NOT NULL DEFAULT '',
    normalized_cmd TEXT NOT NULL DEFAULT '',
    invoker       TEXT NOT NULL DEFAULT '',
    method        TEXT NOT NULL DEFAULT '',
    reason        TEXT NOT NULL DEFAULT '',
    approved_by   TEXT NOT NULL DEFAULT '',
    exit_code     INTEGER DEFAULT NULL,
    exec_log_id   INTEGER DEFAULT NULL,
    started_at    DATETIME NOT NULL,
    ended_at      DATETIME DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_priv_server  ON audit_privilege_events(server_name, started_at);
CREATE INDEX IF NOT EXISTS idx_audit_priv_run     ON audit_privilege_events(run_id);
`

const createAuditFileSnapshotsSQL = `
CREATE TABLE IF NOT EXISTS audit_file_snapshots (
    id                   TEXT PRIMARY KEY,
    privilege_event_id   TEXT NOT NULL REFERENCES audit_privilege_events(id),
    path                 TEXT NOT NULL,
    phase                TEXT NOT NULL DEFAULT 'pre',
    sha256               TEXT NOT NULL DEFAULT '',
    size                 INTEGER NOT NULL DEFAULT 0,
    mode                 TEXT NOT NULL DEFAULT '',
    owner                TEXT NOT NULL DEFAULT '',
    grp                  TEXT NOT NULL DEFAULT '',
    content_blob         BLOB DEFAULT NULL,
    captured_at          DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_snap_event ON audit_file_snapshots(privilege_event_id, phase);
CREATE INDEX IF NOT EXISTS idx_audit_snap_path  ON audit_file_snapshots(path, phase);
`

// AuditPrivilegeEvent는 audit_privilege_events 테이블의 단일 행이다.
type AuditPrivilegeEvent struct {
	ID            string
	RunID         string
	ServerID      string
	ServerName    string
	Command       string
	NormalizedCmd string
	Invoker       string
	Method        string
	Reason        string
	ApprovedBy    string
	ExitCode      *int
	ExecLogID     *int64
	StartedAt     time.Time
	EndedAt       *time.Time
}

// AuditFileSnapshot는 audit_file_snapshots 테이블의 단일 행이다.
type AuditFileSnapshot struct {
	ID               string
	PrivilegeEventID string
	Path             string
	Phase            string // "pre" | "post"
	SHA256           string
	Size             int64
	Mode             string
	Owner            string
	Group            string
	ContentBlob      []byte // gzip 압축 (nil이면 미저장)
	CapturedAt       time.Time
}

// AuditPrivilegeStore는 권한 상승 이벤트 접근 인터페이스이다.
type AuditPrivilegeStore interface {
	InsertAuditPrivilegeEvent(ctx context.Context, evt AuditPrivilegeEvent) error
	UpdateAuditPrivilegeEventEnd(ctx context.Context, id string, exitCode int, endedAt time.Time, execLogID *int64) error
	InsertAuditFileSnapshot(ctx context.Context, snap AuditFileSnapshot) error
	ListAuditFileChanges(ctx context.Context, privilegeEventID string) ([]AuditFileSnapshotPair, error)
}

// AuditFileSnapshotPair는 pre/post 스냅샷 쌍이다.
type AuditFileSnapshotPair struct {
	Path string
	Pre  *AuditFileSnapshot
	Post *AuditFileSnapshot
	Changed bool
}

// initAuditPrivilegeSchema는 권한 상승 및 파일 스냅샷 테이블을 생성한다.
func (s *SQLiteStore) initAuditPrivilegeSchema(ctx context.Context) {
	for _, stmt := range []string{createAuditPrivilegeEventsSQL, createAuditFileSnapshotsSQL} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			slog.Warn("init audit privilege schema", "err", err)
		}
	}
}

// InsertAuditPrivilegeEvent는 권한 상승 이벤트를 삽입한다 (멱등).
func (s *SQLiteStore) InsertAuditPrivilegeEvent(ctx context.Context, evt AuditPrivilegeEvent) error {
	var runID any
	if evt.RunID != "" {
		runID = evt.RunID
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO audit_privilege_events
		 (id, run_id, workspace_id, server_name, command, normalized_cmd, invoker, method, reason, approved_by, exit_code, exec_log_id, started_at, ended_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		evt.ID, runID, evt.ServerID, evt.ServerName,
		evt.Command, evt.NormalizedCmd, evt.Invoker, evt.Method,
		evt.Reason, evt.ApprovedBy, evt.ExitCode, evt.ExecLogID,
		evt.StartedAt.UTC(), nullTime(evt.EndedAt),
	)
	if err != nil {
		return fmt.Errorf("insert audit privilege event: %w", err)
	}
	return nil
}

// UpdateAuditPrivilegeEventEnd는 권한 상승 이벤트의 결과를 기록한다.
func (s *SQLiteStore) UpdateAuditPrivilegeEventEnd(ctx context.Context, id string, exitCode int, endedAt time.Time, execLogID *int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE audit_privilege_events SET exit_code=?, ended_at=?, exec_log_id=? WHERE id=?`,
		exitCode, endedAt.UTC(), execLogID, id,
	)
	if err != nil {
		return fmt.Errorf("update audit privilege event: %w", err)
	}
	return nil
}

// InsertAuditFileSnapshot는 파일 스냅샷을 삽입한다 (멱등). 64KB 이하면 gzip 압축 저장.
func (s *SQLiteStore) InsertAuditFileSnapshot(ctx context.Context, snap AuditFileSnapshot) error {
	var blob any
	if len(snap.ContentBlob) > 0 && len(snap.ContentBlob) <= 64*1024 {
		blob = gzipCompress(snap.ContentBlob)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO audit_file_snapshots
		 (id, privilege_event_id, path, phase, sha256, size, mode, owner, grp, content_blob, captured_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.ID, snap.PrivilegeEventID, snap.Path, snap.Phase,
		snap.SHA256, snap.Size, snap.Mode, snap.Owner, snap.Group,
		blob, snap.CapturedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert audit file snapshot: %w", err)
	}
	return nil
}

// ListAuditFileChanges는 privilege event 의 pre/post 스냅샷 쌍을 반환한다.
func (s *SQLiteStore) ListAuditFileChanges(ctx context.Context, privilegeEventID string) ([]AuditFileSnapshotPair, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, privilege_event_id, path, phase, sha256, size, mode, owner, grp, captured_at
		 FROM audit_file_snapshots WHERE privilege_event_id=? ORDER BY path, phase`, privilegeEventID)
	if err != nil {
		return nil, fmt.Errorf("list audit file snapshots: %w", err)
	}
	defer rows.Close()

	byPath := map[string]*AuditFileSnapshotPair{}
	var order []string
	for rows.Next() {
		var snap AuditFileSnapshot
		if err := rows.Scan(&snap.ID, &snap.PrivilegeEventID, &snap.Path, &snap.Phase,
			&snap.SHA256, &snap.Size, &snap.Mode, &snap.Owner, &snap.Group, &snap.CapturedAt); err != nil {
			return nil, fmt.Errorf("scan audit file snapshot: %w", err)
		}
		pair, ok := byPath[snap.Path]
		if !ok {
			pair = &AuditFileSnapshotPair{Path: snap.Path}
			byPath[snap.Path] = pair
			order = append(order, snap.Path)
		}
		cp := snap
		if snap.Phase == "pre" {
			pair.Pre = &cp
		} else {
			pair.Post = &cp
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit file snapshots: %w", err)
	}

	result := make([]AuditFileSnapshotPair, 0, len(order))
	for _, path := range order {
		pair := *byPath[path]
		if pair.Pre != nil && pair.Post != nil {
			pair.Changed = pair.Pre.SHA256 != pair.Post.SHA256
		} else {
			pair.Changed = true
		}
		result = append(result, pair)
	}
	return result, nil
}

func gzipCompress(data []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := io.Copy(w, bytes.NewReader(data)); err != nil {
		return data
	}
	if err := w.Close(); err != nil {
		return data
	}
	return buf.Bytes()
}

// GetAuditPrivilegeEvent는 ID로 단일 이벤트를 반환한다.
func (s *SQLiteStore) GetAuditPrivilegeEvent(ctx context.Context, id string) (AuditPrivilegeEvent, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, COALESCE(run_id,''), workspace_id, server_name, command, normalized_cmd, invoker, method, reason, approved_by, exit_code, exec_log_id, started_at, ended_at
		 FROM audit_privilege_events WHERE id=?`, id)
	var evt AuditPrivilegeEvent
	var exitCode sql.NullInt64
	var execLogID sql.NullInt64
	var endedAt sql.NullTime
	if err := row.Scan(
		&evt.ID, &evt.RunID, &evt.ServerID, &evt.ServerName,
		&evt.Command, &evt.NormalizedCmd, &evt.Invoker, &evt.Method,
		&evt.Reason, &evt.ApprovedBy, &exitCode, &execLogID,
		&evt.StartedAt, &endedAt,
	); err != nil {
		return AuditPrivilegeEvent{}, fmt.Errorf("get audit privilege event: %w", err)
	}
	if exitCode.Valid {
		v := int(exitCode.Int64)
		evt.ExitCode = &v
	}
	if execLogID.Valid {
		evt.ExecLogID = &execLogID.Int64
	}
	if endedAt.Valid {
		evt.EndedAt = &endedAt.Time
	}
	return evt, nil
}
