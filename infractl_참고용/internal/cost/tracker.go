// Package cost
// File: tracker.go
// Description: LLM 비용/사용량 기록 및 조회 트래커
// Responsibility: 매 LLM 호출마다 토큰 수와 예상 비용을 SQLite에 기록하고 집계

package cost

import (
	"context"
	"log/slog"
)

// Store는 비용 데이터 영속화 인터페이스이다.
// internal/store.CostStore가 이 인터페이스를 구현한다.
type Store interface {
	SaveUsageLog(ctx context.Context, entry UsageEntry) (int64, error)
	AggregateUsage(ctx context.Context, days int) (CostSummary, error)
	DailyUsage(ctx context.Context, days int) ([]DailyCost, error)
}

// Tracker는 LLM 호출 비용을 기록하고 조회하는 트래커이다.
type Tracker struct {
	store     Store
	pricing   map[string]ModelPricing
	modelName string
}

// NewTracker는 트래커를 생성한다.
// modelName은 기본 LLM 모델 이름이다.
// pricing은 모델명→단가 맵이다. nil이면 비용 계산을 건너뛴다.
func NewTracker(store Store, modelName string, pricing map[string]ModelPricing) *Tracker {
	if pricing == nil {
		pricing = make(map[string]ModelPricing)
	}
	return &Tracker{store: store, pricing: pricing, modelName: modelName}
}

// Record는 LLM 호출 1회의 사용량을 비동기 기록한다.
// 기록 실패는 에이전트 루프를 중단하지 않고 WARN 로그만 남긴다.
func (t *Tracker) Record(ctx context.Context, model string, inputTokens, outputTokens int, src Source, sessionID int64) {
	if t == nil || t.store == nil {
		return
	}
	if model == "" {
		model = t.modelName
	}
	cost := t.calcCost(model, inputTokens, outputTokens)
	entry := UsageEntry{
		Model:         model,
		InputTokens:   inputTokens,
		OutputTokens:  outputTokens,
		EstimatedCost: cost,
		Source:        src,
		SessionID:     sessionID,
	}
	go func() {
		if _, err := t.store.SaveUsageLog(ctx, entry); err != nil {
			slog.Warn("cost record failed", "err", err)
		}
	}()
}

// Summary는 지정 일수 기간의 비용 집계를 반환한다.
// days=7이면 최근 7일, days=30이면 최근 30일.
func (t *Tracker) Summary(ctx context.Context, days int) (CostSummary, error) {
	return t.store.AggregateUsage(ctx, days)
}

// DailyCosts는 지정 일수의 일별 비용 목록을 반환한다.
func (t *Tracker) DailyCosts(ctx context.Context, days int) ([]DailyCost, error) {
	return t.store.DailyUsage(ctx, days)
}

// calcCost는 모델별 단가로 예상 비용(USD)을 계산한다.
func (t *Tracker) calcCost(model string, inputTokens, outputTokens int) float64 {
	p, ok := t.pricing[model]
	if !ok {
		return 0
	}
	return (float64(inputTokens)*p.Input + float64(outputTokens)*p.Output) / 1_000_000
}
