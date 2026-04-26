// Package store
// File: audit_query.go
// Description: 감사 이력 조회용 공용 쿼리 (브리핑·상세·리스트)
// Responsibility: AuditRunSummary 리스트 및 AuditRunDetail 상세 조회 제공

package store

import (
	"context"
	"fmt"
	"time"
)

// AuditQueryStore는 감사 이력 조회 인터페이스이다.
type AuditQueryStore interface {
	ListAuditRunSummaries(ctx context.Context, serverName string, since time.Time, limit int) ([]AuditRunSummary, error)
	DescribeAuditRun(ctx context.Context, runID string) (AuditRunDetail, error)
}

// ListAuditRunSummaries는 서버 기준 run 목록을 최신순으로 반환한다.
// 브리핑·audit list CLI 에 사용한다.
func (s *SQLiteStore) ListAuditRunSummaries(ctx context.Context, serverName string, since time.Time, limit int) ([]AuditRunSummary, error) {
	query := `
	SELECT
	    r.id, r.server_name, r.kind, r.title, r.status,
	    r.started_at, r.ended_at,
	    COUNT(st.id)                                                   AS step_total,
	    SUM(CASE WHEN st.status IN ('ok','skipped') THEN 1 ELSE 0 END) AS step_done,
	    COALESCE((SELECT label FROM audit_steps WHERE run_id=r.id ORDER BY seq DESC LIMIT 1), '') AS last_step_lbl
	FROM audit_runs r
	LEFT JOIN audit_steps st ON st.run_id = r.id
	WHERE r.started_at >= ? AND (? = '' OR r.server_name = ?)
	GROUP BY r.id
	ORDER BY r.started_at DESC
	LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, since.UTC(), serverName, serverName, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit run summaries: %w", err)
	}
	defer rows.Close()

	var list []AuditRunSummary
	for rows.Next() {
		var rs AuditRunSummary
		var kind, status string
		var endedAt *time.Time
		if err := rows.Scan(
			&rs.ID, &rs.ServerName, &kind, &rs.Title, &status,
			&rs.StartedAt, &endedAt,
			&rs.StepTotal, &rs.StepDone, &rs.LastStepLbl,
		); err != nil {
			return nil, fmt.Errorf("scan audit run summary: %w", err)
		}
		rs.Kind = AuditRunKind(kind)
		rs.Status = AuditRunStatus(status)
		rs.EndedAt = endedAt
		list = append(list, rs)
	}
	return list, rows.Err()
}

// DescribeAuditRun은 단일 run 의 상세(계획서 + 스텝 목록)를 반환한다.
func (s *SQLiteStore) DescribeAuditRun(ctx context.Context, runID string) (AuditRunDetail, error) {
	run, err := s.GetAuditRun(ctx, runID)
	if err != nil {
		return AuditRunDetail{}, fmt.Errorf("describe audit run: %w", err)
	}

	var activePlan *AuditRunPlan
	if run.ActivePlanID != "" {
		p, err := s.GetAuditRunPlan(ctx, run.ActivePlanID)
		if err == nil {
			activePlan = &p
		}
	}

	var plannedSteps []AuditPlannedStep
	if activePlan != nil {
		plannedSteps, err = s.ListAuditPlannedSteps(ctx, activePlan.ID)
		if err != nil {
			return AuditRunDetail{}, fmt.Errorf("describe audit run planned steps: %w", err)
		}
	}

	steps, err := s.ListAuditSteps(ctx, runID)
	if err != nil {
		return AuditRunDetail{}, fmt.Errorf("describe audit run steps: %w", err)
	}

	return AuditRunDetail{
		Run:          run,
		ActivePlan:   activePlan,
		Steps:        steps,
		PlannedSteps: plannedSteps,
	}, nil
}
