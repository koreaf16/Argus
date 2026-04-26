// Package tui
// File: interfaces.go
// Description: TUI 슬래시 명령에서 사용하는 외부 의존성 인터페이스 정의
// Responsibility: hooks, schedules, checkpoints, cost 명령의 의존성을 인터페이스로 추상화

package tui

import (
	"context"

	"github.com/yourorg/infractl/internal/cost"
	"github.com/yourorg/infractl/internal/store"
)

// HooksManager는 훅 목록 조회, 활성화/비활성화, 삭제 인터페이스이다.
type HooksManager interface {
	List(ctx context.Context) ([]store.Hook, error)
	SetEnabled(ctx context.Context, id int64, enabled bool) error
	Delete(ctx context.Context, id int64) error
}

// ScheduleManager는 스케줄 목록 조회, 활성화/비활성화, 삭제 인터페이스이다.
type ScheduleManager interface {
	List(ctx context.Context) ([]store.Schedule, error)
	SetEnabled(ctx context.Context, id int64, enabled bool) error
	Delete(ctx context.Context, id int64) error
}

// CheckpointManager는 체크포인트 목록 조회 인터페이스이다.
type CheckpointManager interface {
	List(ctx context.Context, server string, limit int) ([]store.Checkpoint, error)
}

// CostTracker는 LLM 비용/사용량 집계 인터페이스이다.
type CostTracker interface {
	Summary(ctx context.Context, days int) (cost.CostSummary, error)
	DailyCosts(ctx context.Context, days int) ([]cost.DailyCost, error)
}
