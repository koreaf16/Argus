// Package store
// File: audit_plan_store.go
// Description: audit_run_plans / audit_planned_steps 테이블 CRUD 및 스키마 초기화
// Responsibility: 작업 계획서 revision 관리 및 계획 스텝 정의 영속화

package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
)

const createAuditRunPlansSQL = `
CREATE TABLE IF NOT EXISTS audit_run_plans (
    id           TEXT PRIMARY KEY,
    run_id       TEXT NOT NULL REFERENCES audit_runs(id),
    revision     INTEGER NOT NULL DEFAULT 1,
    author       TEXT NOT NULL DEFAULT 'llm',
    goal_md      TEXT NOT NULL DEFAULT '',
    rationale_md TEXT NOT NULL DEFAULT '',
    plan_md      TEXT NOT NULL DEFAULT '',
    risk_md      TEXT NOT NULL DEFAULT '',
    approved_by  TEXT NOT NULL DEFAULT '',
    approved_at  DATETIME DEFAULT NULL,
    created_at   DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_plans_run ON audit_run_plans(run_id, revision);
`

const createAuditPlannedStepsSQL = `
CREATE TABLE IF NOT EXISTS audit_planned_steps (
    id            TEXT PRIMARY KEY,
    plan_id       TEXT NOT NULL REFERENCES audit_run_plans(id),
    seq           INTEGER NOT NULL,
    label         TEXT NOT NULL DEFAULT '',
    intent_md     TEXT NOT NULL DEFAULT '',
    expected_cmds TEXT NOT NULL DEFAULT '',
    rollback_md   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_audit_planned_steps_plan ON audit_planned_steps(plan_id, seq);
`

// AuditPlanStore는 계획서 관련 테이블 접근 인터페이스이다.
type AuditPlanStore interface {
	InsertAuditRunPlan(ctx context.Context, plan AuditRunPlan) error
	GetAuditRunPlan(ctx context.Context, id string) (AuditRunPlan, error)
	ListAuditRunPlans(ctx context.Context, runID string) ([]AuditRunPlan, error)
	InsertAuditPlannedStep(ctx context.Context, step AuditPlannedStep) error
	ListAuditPlannedSteps(ctx context.Context, planID string) ([]AuditPlannedStep, error)
}

// initAuditPlanSchema는 계획서·계획 스텝 테이블을 생성한다.
func (s *SQLiteStore) initAuditPlanSchema(ctx context.Context) {
	for _, stmt := range []string{createAuditRunPlansSQL, createAuditPlannedStepsSQL} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			slog.Warn("init audit plan schema", "err", err)
		}
	}
}

// InsertAuditRunPlan은 계획서를 삽입한다 (멱등).
func (s *SQLiteStore) InsertAuditRunPlan(ctx context.Context, plan AuditRunPlan) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO audit_run_plans
		 (id, run_id, revision, author, goal_md, rationale_md, plan_md, risk_md, approved_by, approved_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		plan.ID, plan.RunID, plan.Revision,
		string(plan.Author), plan.GoalMD, plan.RationaleMD,
		plan.PlanMD, plan.RiskMD, plan.ApprovedBy,
		nullTime(plan.ApprovedAt), plan.CreatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert audit run plan: %w", err)
	}
	return nil
}

// GetAuditRunPlan은 ID로 계획서를 조회한다.
func (s *SQLiteStore) GetAuditRunPlan(ctx context.Context, id string) (AuditRunPlan, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, run_id, revision, author, goal_md, rationale_md, plan_md, risk_md, approved_by, approved_at, created_at
		 FROM audit_run_plans WHERE id=?`, id)
	return scanAuditPlan(row)
}

// ListAuditRunPlans는 run의 모든 revision을 오름차순으로 반환한다.
func (s *SQLiteStore) ListAuditRunPlans(ctx context.Context, runID string) ([]AuditRunPlan, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, revision, author, goal_md, rationale_md, plan_md, risk_md, approved_by, approved_at, created_at
		 FROM audit_run_plans WHERE run_id=? ORDER BY revision ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("list audit run plans: %w", err)
	}
	defer rows.Close()
	var plans []AuditRunPlan
	for rows.Next() {
		p, err := scanAuditPlan(rows)
		if err != nil {
			return nil, err
		}
		plans = append(plans, p)
	}
	return plans, rows.Err()
}

// InsertAuditPlannedStep은 계획 스텝을 삽입한다 (멱등).
func (s *SQLiteStore) InsertAuditPlannedStep(ctx context.Context, step AuditPlannedStep) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO audit_planned_steps
		 (id, plan_id, seq, label, intent_md, expected_cmds, rollback_md)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		step.ID, step.PlanID, step.Seq,
		step.Label, step.IntentMD, step.ExpectedCmds, step.RollbackMD,
	)
	if err != nil {
		return fmt.Errorf("insert audit planned step: %w", err)
	}
	return nil
}

// ListAuditPlannedSteps는 계획서의 모든 스텝을 순서대로 반환한다.
func (s *SQLiteStore) ListAuditPlannedSteps(ctx context.Context, planID string) ([]AuditPlannedStep, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, plan_id, seq, label, intent_md, expected_cmds, rollback_md
		 FROM audit_planned_steps WHERE plan_id=? ORDER BY seq ASC`, planID)
	if err != nil {
		return nil, fmt.Errorf("list audit planned steps: %w", err)
	}
	defer rows.Close()
	var steps []AuditPlannedStep
	for rows.Next() {
		var ps AuditPlannedStep
		if err := rows.Scan(&ps.ID, &ps.PlanID, &ps.Seq, &ps.Label, &ps.IntentMD, &ps.ExpectedCmds, &ps.RollbackMD); err != nil {
			return nil, fmt.Errorf("scan audit planned step: %w", err)
		}
		steps = append(steps, ps)
	}
	return steps, rows.Err()
}

type auditPlanScanner interface {
	Scan(dest ...interface{}) error
}

func scanAuditPlan(row auditPlanScanner) (AuditRunPlan, error) {
	var p AuditRunPlan
	var author string
	var approvedAt sql.NullTime
	err := row.Scan(
		&p.ID, &p.RunID, &p.Revision,
		&author, &p.GoalMD, &p.RationaleMD,
		&p.PlanMD, &p.RiskMD, &p.ApprovedBy,
		&approvedAt, &p.CreatedAt,
	)
	if err != nil {
		return AuditRunPlan{}, fmt.Errorf("scan audit run plan: %w", err)
	}
	p.Author = AuditPlanAuthor(author)
	if approvedAt.Valid {
		t := approvedAt.Time
		p.ApprovedAt = &t
	}
	return p, nil
}

// NextAuditPlanRevision은 run의 다음 revision 번호를 반환한다.
func (s *SQLiteStore) NextAuditPlanRevision(ctx context.Context, runID string) (int, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(revision), 0)+1 FROM audit_run_plans WHERE run_id=?`, runID)
	var next int
	if err := row.Scan(&next); err != nil {
		return 1, fmt.Errorf("next audit plan revision: %w", err)
	}
	return next, nil
}

// NextAuditPlannedStepSeq는 plan의 다음 seq 번호를 반환한다.
func (s *SQLiteStore) NextAuditPlannedStepSeq(ctx context.Context, planID string) (int, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0)+1 FROM audit_planned_steps WHERE plan_id=?`, planID)
	var next int
	if err := row.Scan(&next); err != nil {
		return 1, fmt.Errorf("next audit planned step seq: %w", err)
	}
	return next, nil
}

