package agent

import (
	"context"
	"testing"

	"github.com/yourorg/infractl/internal/llm"
)

type noopLLMClient struct{}

func (noopLLMClient) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolDef, _ interface{}, opts ...llm.CallOption) (llm.Response, error) {
	return llm.Response{}, nil
}

func (noopLLMClient) ChatStream(_ context.Context, _ []llm.Message, _ []llm.ToolDef, _ interface{}, _ func(string), _ func(string), opts ...llm.CallOption) (llm.Response, error) {
	return llm.Response{}, nil
}

func newTestAgentWithTiers(tiers ...llm.Tier) *Agent {
	reg := llm.NewRegistry()
	for _, tier := range tiers {
		reg.Register(tier, noopLLMClient{}, string(tier)+"-model")
	}
	return &Agent{llmRegistry: reg}
}

func TestResolveClientForTierRoutesReasoningTier(t *testing.T) {
	agent := newTestAgentWithTiers(llm.TierGeneral, llm.TierReasoning)

	_, got, _ := agent.resolveClientForTier("reasoning")
	if got != llm.TierReasoning {
		t.Fatalf("expected reasoning tier, got %s", got)
	}
}

func TestResolveClientForTierRoutesFastTier(t *testing.T) {
	agent := newTestAgentWithTiers(llm.TierGeneral, llm.TierFast)

	_, got, _ := agent.resolveClientForTier("fast")
	if got != llm.TierFast {
		t.Fatalf("expected fast tier, got %s", got)
	}
}

func TestResolveClientForTierFallsBackToGeneralWhenMissing(t *testing.T) {
	agent := newTestAgentWithTiers(llm.TierGeneral)

	_, got, _ := agent.resolveClientForTier("reasoning")
	if got != llm.TierGeneral {
		t.Fatalf("expected general tier fallback, got %s", got)
	}
}

func TestResolveClientForTierDefaultsToGeneralForUnknownTier(t *testing.T) {
	agent := newTestAgentWithTiers(llm.TierGeneral, llm.TierReasoning)

	_, got, _ := agent.resolveClientForTier("unknown")
	if got != llm.TierGeneral {
		t.Fatalf("expected general for unknown tier, got %s", got)
	}
}

func TestResolveClientForTierUsesLLMClientWithoutRegistry(t *testing.T) {
	agent := &Agent{llmClient: noopLLMClient{}, modelName: "default-model"}

	_, gotTier, gotModel := agent.resolveClientForTier("reasoning")
	if gotTier != llm.TierGeneral {
		t.Fatalf("expected general tier without registry, got %s", gotTier)
	}
	if gotModel != "default-model" {
		t.Fatalf("expected default-model, got %s", gotModel)
	}
}
