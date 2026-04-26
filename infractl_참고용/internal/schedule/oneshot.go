// Package schedule
// File: oneshot.go
// Description: 1회성 스케줄 — 지정 시각 단 1회 실행 후 자동 비활성화
// Responsibility: OneshotManager — time.AfterFunc 기반 oneshot 등록/복원/취소

package schedule

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/yourorg/infractl/internal/store"
)

// OneshotManager는 1회성 스케줄을 time.AfterFunc으로 관리한다.
// 프로세스 재시작 시 RestoreFromStore로 미완료 타이머를 재등록한다.
type OneshotManager struct {
	mu     sync.Mutex
	store  store.ScheduleStore
	runner AgentRunner
	logger *Logger
	timers map[int64]*time.Timer // schedule id → timer
}

// NewOneshotManager는 OneshotManager를 생성한다.
// logger가 nil이면 실행 결과를 로그에 기록하지 않는다.
func NewOneshotManager(st store.ScheduleStore, runner AgentRunner, logger *Logger) *OneshotManager {
	return &OneshotManager{
		store:  st,
		runner: runner,
		logger: logger,
		timers: make(map[int64]*time.Timer),
	}
}

// Add는 새 oneshot 스케줄을 저장하고 타이머를 등록한다.
// at가 현재 시각 이전이면 즉시 실행한다.
func (m *OneshotManager) Add(ctx context.Context, at time.Time, name, prompt string) (int64, error) {
	sch := store.Schedule{
		Name:    name,
		Prompt:  prompt,
		Enabled: true,
		Oneshot: true,
		RunAt:   &at,
	}
	id, err := m.store.SaveSchedule(ctx, sch)
	if err != nil {
		return 0, fmt.Errorf("save oneshot %s: %w", name, err)
	}

	m.scheduleTimer(id, name, prompt, at)
	slog.Info("oneshot added", "id", id, "name", name, "run_at", at.Format(time.RFC3339))
	return id, nil
}

// RestoreFromStore는 저장소에서 미완료 oneshot(Enabled=true)을 로드해 타이머를 재등록한다.
// 프로세스 재시작 시 Scheduler.Start에서 호출한다.
func (m *OneshotManager) RestoreFromStore(ctx context.Context) error {
	schedules, err := m.store.ListSchedules(ctx)
	if err != nil {
		return fmt.Errorf("list schedules for oneshot restore: %w", err)
	}
	restored := 0
	for _, s := range schedules {
		if !s.Oneshot || !s.Enabled || s.RunAt == nil {
			continue
		}
		m.scheduleTimer(s.ID, s.Name, s.Prompt, *s.RunAt)
		restored++
	}
	if restored > 0 {
		slog.Info("restored oneshot timers", "count", restored)
	}
	return nil
}

// Cancel은 진행 중인 oneshot 타이머를 취소한다.
// 이미 실행 중인 goroutine은 중단하지 않는다.
func (m *OneshotManager) Cancel(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.timers[id]; ok {
		t.Stop()
		delete(m.timers, id)
	}
}

// scheduleTimer는 at 시각에 id/name/prompt를 실행하는 타이머를 등록한다.
func (m *OneshotManager) scheduleTimer(id int64, name, prompt string, at time.Time) {
	delay := max(time.Until(at), 0) // 과거 시각이면 즉시 실행
	t := time.AfterFunc(delay, func() {
		m.run(id, name, prompt)
	})
	m.mu.Lock()
	m.timers[id] = t
	m.mu.Unlock()
}

// run은 oneshot 작업을 실행하고 완료 후 비활성화(Enabled=false)한다.
func (m *OneshotManager) run(id int64, name, prompt string) {
	m.mu.Lock()
	delete(m.timers, id)
	m.mu.Unlock()

	ctx := context.Background()

	result, execErr := m.runner(ctx, prompt)
	if execErr != nil {
		slog.Error("oneshot execution failed", "id", id, "name", name, "err", execErr)
	} else {
		slog.Info("oneshot executed", "id", id, "name", name)
	}

	if m.logger != nil {
		m.logger.Write(name, prompt, result, execErr)
	}

	if err := m.store.UpdateLastRun(ctx, id, result); err != nil {
		slog.Warn("oneshot update last run", "id", id, "err", err)
	}
	// 완료 마킹 — Enabled=false로 전환해 PruneOneshotSchedules 대상이 됨
	if err := m.store.SetScheduleEnabled(ctx, id, false); err != nil {
		slog.Warn("oneshot disable after run", "id", id, "err", err)
	}
	slog.Info("oneshot completed", "id", id, "name", name)
}
