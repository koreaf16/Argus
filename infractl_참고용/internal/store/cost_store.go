// Package store
// File: cost_store.go
// Description: 비용/사용량 로그 저장소 인터페이스 및 SQLite 구현
// Responsibility: usage_logs 테이블 CRUD 및 집계 쿼리 제공

package store

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/yourorg/infractl/internal/cost"
)

// CostStore는 LLM 사용량/비용 영속화 인터페이스이다.
type CostStore interface {
	SaveUsageLog(ctx context.Context, entry cost.UsageEntry) (int64, error)
	AggregateUsage(ctx context.Context, days int) (cost.CostSummary, error)
	DailyUsage(ctx context.Context, days int) ([]cost.DailyCost, error)
}

const createCostSQL = `
CREATE TABLE IF NOT EXISTS usage_logs (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    model          TEXT    NOT NULL,
    input_tokens   INTEGER NOT NULL DEFAULT 0,
    output_tokens  INTEGER NOT NULL DEFAULT 0,
    estimated_cost REAL    NOT NULL DEFAULT 0.0,
    source         TEXT    NOT NULL DEFAULT 'user',
    session_id     INTEGER NOT NULL DEFAULT 0,
    timestamp      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_usage_timestamp ON usage_logs(timestamp);
CREATE INDEX IF NOT EXISTS idx_usage_source    ON usage_logs(source);
`

// initCostSchema는 usage_logs 테이블을 생성한다.
func (s *SQLiteStore) initCostSchema(ctx context.Context) {
	if _, err := s.db.ExecContext(ctx, createCostSQL); err != nil {
		slog.Warn("init cost schema", "err", err)
	}
}

// SaveUsageLog는 LLM 사용량 로그를 저장한다.
func (s *SQLiteStore) SaveUsageLog(ctx context.Context, entry cost.UsageEntry) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO usage_logs (model, input_tokens, output_tokens, estimated_cost, source, session_id, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.Model, entry.InputTokens, entry.OutputTokens, entry.EstimatedCost,
		string(entry.Source), entry.SessionID, time.Now(),
	)
	if err != nil {
		return 0, fmt.Errorf("save usage log: %w", err)
	}
	return res.LastInsertId()
}

// AggregateUsage는 지정 일수 기간의 비용 합계를 반환한다.
func (s *SQLiteStore) AggregateUsage(ctx context.Context, days int) (cost.CostSummary, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT
			COALESCE(SUM(input_tokens),   0),
			COALESCE(SUM(output_tokens),  0),
			COALESCE(SUM(estimated_cost), 0.0),
			COUNT(*)
		 FROM usage_logs
		 WHERE timestamp >= datetime('now', ? || ' days')`,
		fmt.Sprintf("-%d", days),
	)
	var s2 cost.CostSummary
	if err := row.Scan(&s2.TotalInputTokens, &s2.TotalOutputTokens, &s2.TotalCost, &s2.CallCount); err != nil {
		return cost.CostSummary{}, fmt.Errorf("aggregate usage: %w", err)
	}
	return s2, nil
}

// DailyUsage는 지정 일수의 일별 비용 목록을 반환한다.
func (s *SQLiteStore) DailyUsage(ctx context.Context, days int) ([]cost.DailyCost, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT
			date(timestamp) AS day,
			COALESCE(SUM(input_tokens),   0),
			COALESCE(SUM(output_tokens),  0),
			COALESCE(SUM(estimated_cost), 0.0),
			COUNT(*)
		 FROM usage_logs
		 WHERE timestamp >= datetime('now', ? || ' days')
		 GROUP BY day
		 ORDER BY day DESC`,
		fmt.Sprintf("-%d", days),
	)
	if err != nil {
		return nil, fmt.Errorf("daily usage: %w", err)
	}
	defer rows.Close()

	var result []cost.DailyCost
	for rows.Next() {
		var d cost.DailyCost
		if err := rows.Scan(&d.Date, &d.InputTokens, &d.OutputTokens, &d.EstimatedCost, &d.CallCount); err != nil {
			return nil, fmt.Errorf("scan daily cost: %w", err)
		}
		result = append(result, d)
	}
	return result, nil
}
