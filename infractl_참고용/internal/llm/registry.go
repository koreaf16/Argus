// Package llm
// File: registry.go
// Description: 멀티 LLM 클라이언트 레지스트리 — 티어별 클라이언트 등록 및 폴백 해결
// Responsibility: 티어별 Client 등록과 FallbackOrder에 따른 Client 조회 제공

package llm

import (
	"fmt"
	"log/slog"
	"sync"
)

// Registry는 티어별 LLM 클라이언트를 관리한다.
type Registry struct {
	mu         sync.RWMutex
	clients    map[Tier]Client
	modelNames map[Tier]string // 비용 추적용 모델명
}

// NewRegistry는 빈 Registry를 생성한다.
func NewRegistry() *Registry {
	return &Registry{
		clients:    make(map[Tier]Client),
		modelNames: make(map[Tier]string),
	}
}

// Register는 티어에 클라이언트와 모델명을 등록한다.
func (r *Registry) Register(tier Tier, client Client, modelName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[tier] = client
	r.modelNames[tier] = modelName
	slog.Info("llm client registered", "tier", tier, "model", modelName)
}

// Resolve는 요청된 티어의 클라이언트를 반환한다. 해당 티어가 없으면 FallbackOrder에 따라 폴백한다.
// 등록된 클라이언트가 하나도 없으면 에러를 반환한다.
func (r *Registry) Resolve(requested Tier) (Client, Tier, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, tier := range FallbackOrder(requested) {
		if c, ok := r.clients[tier]; ok {
			if tier != requested {
				slog.Debug("llm tier fallback", "requested", requested, "resolved", tier)
			}
			return c, tier, nil
		}
	}
	return nil, "", fmt.Errorf("no llm client registered for tier %s or any fallback", requested)
}

// MustGeneral은 general 티어 클라이언트를 반환한다. 없으면 패닉한다.
func (r *Registry) MustGeneral() Client {
	c, _, err := r.Resolve(TierGeneral)
	if err != nil {
		panic("general llm client is not registered: " + err.Error())
	}
	return c
}

// Has는 해당 티어에 클라이언트가 등록되어 있는지 확인한다.
func (r *Registry) Has(tier Tier) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.clients[tier]
	return ok
}

// ModelName은 티어에 등록된 모델명을 반환한다. 미등록 시 빈 문자열을 반환한다.
func (r *Registry) ModelName(tier Tier) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.modelNames[tier]
}

// Tiers는 등록된 모든 티어 목록을 반환한다.
func (r *Registry) Tiers() []Tier {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tiers := make([]Tier, 0, len(r.clients))
	for t := range r.clients {
		tiers = append(tiers, t)
	}
	return tiers
}
