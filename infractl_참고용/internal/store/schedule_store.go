// Package store
// File: schedule_store.go
// Description: 스케줄 타입 정의, 저장소 인터페이스, SQLite 구현
// Responsibility: schedules 테이블 CRUD 및 활성 스케줄 조회 제공 (oneshot + run_at 지원)

package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// Schedule은 정기 실행 또는 1회성 작업 정의이다.
type Schedule struct {
	ID         int64
	Name       string
	CronExpr   string
	Prompt     string
	LastRun    *time.Time
	LastResult string
	Enabled    bool
	CreatedAt  time.Time
	// Phase F: oneshot 지원
	Oneshot bool       // true이면 1회 실행 후 Enabled=false 로 전환
	RunAt   *time.Time // oneshot 실행 시각 (nil이면 cron 스케줄)
}

// ScheduleStore는 Schedule 영속화 인터페이스이다.
type ScheduleStore interface {
	SaveSchedule(ctx context.Context, s Schedule) (int64, error)
	GetSchedule(ctx context.Context, id int64) (Schedule, error)
	GetScheduleByName(ctx context.Context, name string) (Schedule, error)
	ListSchedules(ctx context.Context) ([]Schedule, error)
	SetScheduleEnabled(ctx context.Context, id int64, enabled bool) error
	UpdateLastRun(ctx context.Context, id int64, result string) error
	DeleteSchedule(ctx context.Context, id int64) error
}

const createScheduleSQL = `
CREATE TABLE IF NOT EXISTS schedules (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL UNIQUE,
    cron_expr   TEXT    NOT NULL DEFAULT '',
    prompt      TEXT    NOT NULL,
    last_run    DATETIME,
    last_result TEXT    NOT NULL DEFAULT '',
    enabled     INTEGER NOT NULL DEFAULT 1,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    oneshot     INTEGER NOT NULL DEFAULT 0,
    run_at      DATETIME
);
`

// initScheduleSchema는 schedules 테이블을 생성하고 신규 컬럼을 idempotent하게 추가한다.
func (s *SQLiteStore) initScheduleSchema(ctx context.Context) {
	if _, err := s.db.ExecContext(ctx, createScheduleSQL); err != nil {
		slog.Warn("init schedule schema", "err", err)
	}
	// 기존 DB에 컬럼이 없는 경우를 위한 idempotent 마이그레이션 (오류 무시)
	s.db.ExecContext(ctx, `ALTER TABLE schedules ADD COLUMN oneshot INTEGER NOT NULL DEFAULT 0`)
	s.db.ExecContext(ctx, `ALTER TABLE schedules ADD COLUMN run_at DATETIME`)
}

// SaveSchedule은 스케줄을 저장하고 ID를 반환한다.
func (s *SQLiteStore) SaveSchedule(ctx context.Context, sch Schedule) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO schedules (name, cron_expr, prompt, enabled, created_at, oneshot, run_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sch.Name, sch.CronExpr, sch.Prompt,
		boolToInt(sch.Enabled), time.Now(),
		boolToInt(sch.Oneshot), timePtr(sch.RunAt),
	)
	if err != nil {
		return 0, fmt.Errorf("save schedule: %w", err)
	}
	return res.LastInsertId()
}

// GetSchedule은 ID로 스케줄을 조회한다.
func (s *SQLiteStore) GetSchedule(ctx context.Context, id int64) (Schedule, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, cron_expr, prompt, last_run, last_result, enabled, created_at, oneshot, run_at
		 FROM schedules WHERE id = ?`, id)
	return scanSchedule(row.Scan)
}

// GetScheduleByName은 이름으로 스케줄을 조회한다.
func (s *SQLiteStore) GetScheduleByName(ctx context.Context, name string) (Schedule, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, cron_expr, prompt, last_run, last_result, enabled, created_at, oneshot, run_at
		 FROM schedules WHERE name = ?`, name)
	return scanSchedule(row.Scan)
}

// ListSchedules는 모든 스케줄을 반환한다.
func (s *SQLiteStore) ListSchedules(ctx context.Context) ([]Schedule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, cron_expr, prompt, last_run, last_result, enabled, created_at, oneshot, run_at
		 FROM schedules ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	defer rows.Close()

	var result []Schedule
	for rows.Next() {
		sch, err := scanSchedule(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan schedule: %w", err)
		}
		result = append(result, sch)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schedules: %w", err)
	}
	return result, nil
}

// SetScheduleEnabled는 스케줄의 활성/비활성 상태를 변경한다.
func (s *SQLiteStore) SetScheduleEnabled(ctx context.Context, id int64, enabled bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE schedules SET enabled = ? WHERE id = ?`, boolToInt(enabled), id)
	if err != nil {
		return fmt.Errorf("set schedule enabled: %w", err)
	}
	return nil
}

// UpdateLastRun은 마지막 실행 시간과 결과를 업데이트한다.
func (s *SQLiteStore) UpdateLastRun(ctx context.Context, id int64, result string) error {
	now := time.Now()
	_, err := s.db.ExecContext(ctx,
		`UPDATE schedules SET last_run = ?, last_result = ? WHERE id = ?`,
		now, result, id)
	if err != nil {
		return fmt.Errorf("update last run: %w", err)
	}
	return nil
}

// DeleteSchedule은 ID로 스케줄을 삭제한다.
func (s *SQLiteStore) DeleteSchedule(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM schedules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete schedule: %w", err)
	}
	return nil
}

// scanSchedule은 Scan 함수로 Schedule을 구성한다.
func scanSchedule(scan func(dest ...any) error) (Schedule, error) {
	var sch Schedule
	var enabled, oneshot int
	var lastRun, runAt sql.NullTime
	if err := scan(
		&sch.ID, &sch.Name, &sch.CronExpr, &sch.Prompt,
		&lastRun, &sch.LastResult, &enabled, &sch.CreatedAt,
		&oneshot, &runAt,
	); err != nil {
		return Schedule{}, fmt.Errorf("scan schedule: %w", err)
	}
	sch.Enabled = enabled == 1
	sch.Oneshot = oneshot == 1
	if lastRun.Valid {
		sch.LastRun = &lastRun.Time
	}
	if runAt.Valid {
		sch.RunAt = &runAt.Time
	}
	return sch, nil
}

// timePtr는 *time.Time을 sql 친화적 any로 변환한다.
// nil이면 nil을 반환해 NULL로 저장한다.
func timePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}
