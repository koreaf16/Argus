// Package llm
// File: router.go
// Description: LLM 라우터 — 작업 복잡도 분류 후 적절한 티어의 Client 반환
// Responsibility: general LLM으로 작업 복잡도를 분류하고 티어별 Client를 선택

package llm

import (
	"context"
	"log/slog"
	"strings"
)

// Router는 작업 내용을 분석하여 적절한 LLM 티어의 클라이언트를 반환한다.
// 에이전트 메인 루프는 항상 general 티어가 담당하며,
// 이 라우터는 서브에이전트나 명시적 위임 시에만 사용한다.
type Router struct {
	registry *Registry
}

// NewRouter는 Registry를 기반으로 Router를 생성한다.
func NewRouter(registry *Registry) *Router {
	return &Router{registry: registry}
}

// Route는 작업 내용을 general LLM이 분류하여 최적의 티어 클라이언트를 반환한다.
// 등록된 티어가 general 하나뿐이면 분류 과정을 생략한다.
func (r *Router) Route(ctx context.Context, task string) (Client, Tier, error) {
	hasReasoning := r.registry.Has(TierReasoning)
	hasFast := r.registry.Has(TierFast)

	// general 단독 모드: 분류 없이 바로 반환
	if !hasReasoning && !hasFast {
		return r.registry.Resolve(TierGeneral)
	}

	tier := r.classify(ctx, task, hasReasoning, hasFast)
	return r.registry.Resolve(tier)
}

// RouteDirectly는 분류 없이 지정 티어의 클라이언트를 반환한다 (폴백 포함).
func (r *Router) RouteDirectly(tier Tier) (Client, Tier, error) {
	return r.registry.Resolve(tier)
}

// classify는 general LLM을 사용해 작업의 티어를 결정한다.
// 분류에 실패하면 general을 반환한다.
func (r *Router) classify(ctx context.Context, task string, hasReasoning, hasFast bool) Tier {
	general, _, err := r.registry.Resolve(TierGeneral)
	if err != nil {
		return TierGeneral
	}

	prompt := buildClassificationPrompt(task, hasReasoning, hasFast)
	systemPrompt := buildSystemPrompt(hasReasoning, hasFast)

	resp, err := general.Chat(ctx, []Message{
		{Role: RoleSystem, Content: systemPrompt},
		{Role: RoleUser, Content: prompt},
	}, nil, nil)
	if err != nil {
		slog.Warn("llm tier classification failed, using general", "err", err)
		return TierGeneral
	}

	return parseTier(resp.Content, hasReasoning, hasFast)
}

// buildSystemPrompt는 등록된 티어에 따라 분류 시스템 프롬프트를 동적으로 생성한다.
func buildSystemPrompt(hasReasoning, hasFast bool) string {
	if hasReasoning && hasFast {
		return `You are a task complexity classifier.
Respond with exactly one word: "reasoning", "general", or "fast".
- reasoning: multi-step analysis, complex debugging, architecture decisions, root cause analysis
- general: standard operations, configuration, moderate complexity queries
- fast: simple lookups, status checks, formatting, yes/no questions, registration/CRUD with known parameters`
	}
	// 2대 모드: reasoning + general (fast 없음)
	return `You are a task complexity classifier.
Respond with exactly one word: "reasoning" or "general".
- reasoning: multi-step analysis, complex debugging, architecture decisions, root cause analysis
- general: everything else`
}

// buildClassificationPrompt는 분류 요청 프롬프트를 생성한다.
func buildClassificationPrompt(task string, hasReasoning, hasFast bool) string {
	if hasReasoning && hasFast {
		return "Classify this task (respond with one word — reasoning/general/fast):\n" + task
	}
	return "Classify this task (respond with one word — reasoning/general):\n" + task
}

// parseTier는 LLM 응답 문자열에서 Tier를 추출한다.
func parseTier(content string, hasReasoning, hasFast bool) Tier {
	lower := strings.ToLower(strings.TrimSpace(content))
	if hasReasoning && strings.Contains(lower, "reasoning") {
		return TierReasoning
	}
	if hasFast && strings.Contains(lower, "fast") {
		return TierFast
	}
	return TierGeneral
}
