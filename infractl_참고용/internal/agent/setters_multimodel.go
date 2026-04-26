// Package agent
// File: setters_multimodel.go
// Description: 멀티 LLM 레지스트리 주입 세터 및 분류 결과 기반 LLM 티어 라우팅
// Responsibility: Agent에 llmRegistry를 주입하고 tier 문자열로 적합한 LLM 클라이언트를 선택

package agent

import (
	"log/slog"

	"github.com/yourorg/infractl/internal/agent/planmode"
	"github.com/yourorg/infractl/internal/llm"
)

// SetLLMRegistry는 멀티 LLM 티어 레지스트리를 주입한다.
// 주입하면 Pass 1 분류 결과의 tier에 따라 티어별 클라이언트를 선택할 수 있다.
func (a *Agent) SetLLMRegistry(reg *llm.Registry) {
	a.llmRegistry = reg
}

// resolveClientForTier는 Pass 1 분류 결과의 tier 문자열로 적합한 LLM 클라이언트를 반환한다.
// 요청된 티어가 등록되지 않으면 General 티어로 폴백한다.
// General도 없으면 기본 llmClient를 반환한다.
func (a *Agent) resolveClientForTier(tier string) (llm.Client, llm.Tier, string) {
	if a.llmRegistry == nil {
		return a.llmClient, llm.TierGeneral, a.modelName
	}

	var wantTier llm.Tier
	switch tier {
	case "fast":
		wantTier = llm.TierFast
	case "reasoning":
		wantTier = llm.TierReasoning
	default:
		wantTier = llm.TierGeneral
	}

	if client, resolvedTier, err := a.llmRegistry.Resolve(wantTier); err == nil {
		modelName := a.llmRegistry.ModelName(resolvedTier)
		slog.Info("tier resolved from classification",
			"requested", tier, "resolved", resolvedTier, "model", modelName)
		return client, resolvedTier, modelName
	}

	// 폴백: General
	if client, resolvedTier, err := a.llmRegistry.Resolve(llm.TierGeneral); err == nil {
		modelName := a.llmRegistry.ModelName(resolvedTier)
		slog.Debug("tier fallback to general", "requested", tier, "model", modelName)
		return client, resolvedTier, modelName
	}

	return a.llmClient, llm.TierGeneral, a.modelName
}

// SetPlanMode는 Plan 모드를 활성화/비활성화한다.
func (a *Agent) SetPlanMode(enabled bool) {
	if a.planState == nil {
		return
	}
	if enabled {
		a.planState.Enter()
	} else {
		a.planState.Exit()
	}
}

// IsPlanMode는 Plan 모드 활성화 여부를 반환한다.
func (a *Agent) IsPlanMode() bool {
	if a.planState == nil {
		return false
	}
	return a.planState.IsActive()
}

// TogglePlanMode는 Plan 모드를 토글하고 변경 후 상태를 반환한다.
func (a *Agent) TogglePlanMode() bool {
	if a.planState == nil {
		return false
	}
	return a.planState.Toggle()
}

// PlanState 는 Plan Mode 상태 객체를 반환한다 (TUI approval gate 에서 참조).
func (a *Agent) PlanState() *planmode.State {
	return a.planState
}
