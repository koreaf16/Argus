// Package agent
// File: learning.go
// Description: 적응형 학습 매니저 — 학습된 시스템 목록 시스템 프롬프트 주입
// Responsibility: LearnedSystemStore 조회, 시스템 프롬프트에 학습된 시스템 정보 제공

package agent

import (
	"context"
	"log/slog"

	"github.com/yourorg/infractl/internal/store"
)

// AdaptiveLearner는 학습된 시스템 정보를 조회하여 시스템 프롬프트에 주입한다.
//
// 실제 학습 발동은 LLM이 web_search + save_learned_system 도구를 통해 자율 수행한다.
// 시스템 프롬프트의 "Adaptive Service Learning" 가이드라인이 LLM 행동을 유도하며,
// 이 컴포넌트는 이미 학습된 시스템 목록을 매 요청마다 프롬프트에 주입하는 역할만 한다.
type AdaptiveLearner struct {
	store store.LearnedSystemStore
}

// NewAdaptiveLearner는 AdaptiveLearner를 생성한다.
func NewAdaptiveLearner(s store.LearnedSystemStore) *AdaptiveLearner {
	return &AdaptiveLearner{store: s}
}

// ListSystems는 학습된 시스템 목록을 반환한다.
// 오류 시 빈 슬라이스를 반환하여 에이전트 동작에 영향을 주지 않는다.
func (al *AdaptiveLearner) ListSystems(ctx context.Context) []store.LearnedSystem {
	systems, err := al.store.ListLearnedSystems(ctx)
	if err != nil {
		slog.Debug("list learned systems", "err", err)
		return nil
	}
	return systems
}
