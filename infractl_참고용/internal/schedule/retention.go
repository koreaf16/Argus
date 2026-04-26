// Package schedule
// File: retention.go
// Description: 완료된 oneshot 스케줄 보관 정책 적용
// Responsibility: PruneOneshotSchedules — keep_days + max_results 초과 항목 삭제

package schedule

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/yourorg/infractl/internal/store"
)

// RetentionConfig는 oneshot 스케줄 보관 정책이다.
type RetentionConfig struct {
	KeepDays   int // 완료 후 보관 일수 (0 = 무제한)
	MaxResults int // 최대 보관 개수 (0 = 무제한)
}

// pruneEntry는 정리 대상 후보이다.
type pruneEntry struct {
	id          int64
	completedAt time.Time
}

// PruneOneshotSchedules는 완료된 oneshot 스케줄 중 정책을 초과한 항목을 삭제한다.
// 완료 기준: Oneshot=true && Enabled=false.
// KeepDays와 MaxResults를 동시에 적용하며, 어느 한쪽이라도 초과하면 삭제한다.
func PruneOneshotSchedules(ctx context.Context, st store.ScheduleStore, cfg RetentionConfig) error {
	schedules, err := st.ListSchedules(ctx)
	if err != nil {
		return fmt.Errorf("list schedules for prune: %w", err)
	}

	var candidates []pruneEntry
	for _, s := range schedules {
		if !s.Oneshot || s.Enabled {
			continue // 미완료 또는 cron 스케줄은 건드리지 않음
		}
		completedAt := s.CreatedAt
		if s.LastRun != nil {
			completedAt = *s.LastRun
		}
		candidates = append(candidates, pruneEntry{s.ID, completedAt})
	}

	// 오래된 것부터 정렬 (가장 오래된 항목이 index 0)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].completedAt.Before(candidates[j].completedAt)
	})

	cutoff := time.Now().AddDate(0, 0, -cfg.KeepDays)
	deleted := 0

	for i, c := range candidates {
		shouldDelete := false
		if cfg.KeepDays > 0 && c.completedAt.Before(cutoff) {
			shouldDelete = true
		}
		if cfg.MaxResults > 0 && (len(candidates)-i) > cfg.MaxResults {
			shouldDelete = true
		}
		if !shouldDelete {
			continue
		}
		if err := st.DeleteSchedule(ctx, c.id); err != nil {
			slog.Warn("prune oneshot", "id", c.id, "err", err)
		} else {
			deleted++
		}
	}

	if deleted > 0 {
		slog.Info("pruned oneshot schedules", "deleted", deleted)
	}
	return nil
}
