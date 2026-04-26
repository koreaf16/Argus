// Package store
// File: audit_step_store.go
// Description: audit_steps 테이블 CRUD 및 스키마 초기화
// Responsibility: 실행 스텝 이력 영속화 (execution_logs 와 연결)

package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

const createAuditStepsSQL = `
CREATE TABLE IF NOT EXISTS audit_steps (
    id               TEXT PRIMARY KEY,
    run_id           TEXT NOT NULL REFERENCES audit_runs(id),
    planned_step_id  TEXT DEFAULT NULL,
    seq              INTEGER NOT NULL DEFAULT 0,
    label            TEXT NOT NULL DEFAULT '',
    exec_log_id      INTEGER DEFAULT NULL,
    status           TEXT NOT NULL DEFAULT 'ok',
    result_md        TEXT NOT NULL DEFAULT '',
    deviation_md     TEXT NOT NULL DEFAULT '',
    started_at       DATETIME NOT NULL,
    ended_at         DATETIME DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_steps_run ON audit_steps(run_id, seq);
`

// AuditStepStore는 audit_steps 테이블 접근 인터페이스이다.
type AuditStepStore interface {
	InsertAuditStep(ctx context.Context, step AuditStep) error
	UpdateAuditStepStatus(ctx context.Context, id string, status AuditStepStatus, endedAt time.Time, resultMD string) error
	ListAuditSteps(ctx context.Context, runID string) ([]AuditStep, error)
	NextAuditStepSeq(ctx context.Context, runID string) (int, error)
}

// initAuditStepSchema는 audit_steps 테이블과 인덱스를 생성한다.
func (s *SQLiteStore) initAuditStepSchema(ctx context.Context) {
	if _, err := s.db.ExecContext(ctx, createAuditStepsSQL); err != nil {
		slog.Warn("init audit_steps schema", "err", err)
	}
}

// InsertAuditStep은 실행 스텝을 삽입한다 (멱등).
func (s *SQLiteStore) InsertAuditStep(ctx context.Context, step AuditStep) error {
	var execLogID interface{}
	if step.ExecLogID != nil {
		execLogID = *step.ExecLogID
	}
	var plannedStepID interface{}
	if step.PlannedStepID != "" {
		plannedStepID = step.PlannedStepID
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO audit_steps
		 (id, run_id, planned_step_id, seq, label, exec_log_id, status, result_md, deviation_md, started_at, ended_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		step.ID, step.RunID, plannedStepID,
		step.Seq, step.Label, execLogID,
		string(step.Status), step.ResultMD, step.DeviationMD,
		step.StartedAt.UTC(), nullTime(step.EndedAt),
	)
	if err != nil {
		return fmt.Errorf("insert audit step: %w", err)
	}
	return nil
}

// UpdateAuditStepStatus는 스텝의 상태와 종료 시각을 갱신한다.
func (s *SQLiteStore) UpdateAuditStepStatus(ctx context.Context, id string, status AuditStepStatus, endedAt time.Time, resultMD string) error {
	t := endedAt.UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE audit_steps SET status=?, ended_at=?, result_md=? WHERE id=?`,
		string(status), t, resultMD, id,
	)
	if err != nil {
		return fmt.Errorf("update audit step status %s: %w", id, err)
	}
	return nil
}

// ListAuditSteps는 run 의 모든 스텝을 seq 순으로 반환한다.
func (s *SQLiteStore) ListAuditSteps(ctx context.Context, runID string) ([]AuditStep, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, COALESCE(planned_step_id,''), seq, label, exec_log_id,
		        status, result_md, deviation_md, started_at, ended_at
		 FROM audit_steps WHERE run_id=? ORDER BY seq ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("list audit steps: %w", err)
	}
	defer rows.Close()
	var steps []AuditStep
	for rows.Next() {
		st, err := scanAuditStep(rows)
		if err != nil {
			return nil, err
		}
		steps = append(steps, st)
	}
	return steps, rows.Err()
}

// NextAuditStepSeq는 run 의 다음 seq 번호를 반환한다.
func (s *SQLiteStore) NextAuditStepSeq(ctx context.Context, runID string) (int, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0)+1 FROM audit_steps WHERE run_id=?`, runID)
	var next int
	if err := row.Scan(&next); err != nil {
		return 1, fmt.Errorf("next audit step seq: %w", err)
	}
	return next, nil
}

type auditStepScanner interface {
	Scan(dest ...interface{}) error
}

func scanAuditStep(row auditStepScanner) (AuditStep, error) {
	var st AuditStep
	var status string
	var execLogID sql.NullInt64
	var endedAt sql.NullTime
	err := row.Scan(
		&st.ID, &st.RunID, &st.PlannedStepID,
		&st.Seq, &st.Label, &execLogID,
		&status, &st.ResultMD, &st.DeviationMD,
		&st.StartedAt, &endedAt,
	)
	if err != nil {
		return AuditStep{}, fmt.Errorf("scan audit step: %w", err)
	}
	st.Status = AuditStepStatus(status)
	if execLogID.Valid {
		st.ExecLogID = &execLogID.Int64
	}
	if endedAt.Valid {
		st.EndedAt = &endedAt.Time
	}
	return st, nil
}
