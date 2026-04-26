package agent

import (
	"fmt"
	"strings"
)

type promptProfile struct {
	Mode                 string
	Model                string
	NeedsTools           bool
	PromptCacheHit       bool
	TotalBytes           int
	PrefixBytes          int
	TaskMemoryBytes      int
	BeforeKnowledgeBytes int
	KnowledgeBytes       int
	AfterKnowledgeBytes  int
	ResolutionBytes      int
	PlanBytes            int
	ToolCount            int
	SectionCount         int
	ServerCount          int
	ConnectorCount       int
	LearnedSystemCount   int
	RAGSourceCount       int
}

func (a *Agent) recordPromptProfile(p promptProfile) {
	a.lastPromptProfile = p
}

func (a *Agent) PromptDebugReport() string {
	var sb strings.Builder
	p := a.lastPromptProfile
	cacheStats := promptCacheStats{}
	if a.promptCache != nil {
		cacheStats = a.promptCache.Stats()
	}
	inputStats := promptInputCacheStats{}
	if a.promptInputs != nil {
		inputStats = a.promptInputs.Stats()
	}

	sb.WriteString("Last prompt:\n")
	sb.WriteString(fmt.Sprintf("  mode: %s\n", p.Mode))
	sb.WriteString(fmt.Sprintf("  model: %s\n", p.Model))
	sb.WriteString(fmt.Sprintf("  needs_tools: %t\n", p.NeedsTools))
	sb.WriteString(fmt.Sprintf("  prompt_cache_hit: %t\n", p.PromptCacheHit))
	sb.WriteString(fmt.Sprintf("  total_bytes: %d\n", p.TotalBytes))
	sb.WriteString(fmt.Sprintf("  prefix_bytes: %d\n", p.PrefixBytes))
	sb.WriteString(fmt.Sprintf("  task_memory_bytes: %d\n", p.TaskMemoryBytes))
	sb.WriteString(fmt.Sprintf("  before_knowledge_bytes: %d\n", p.BeforeKnowledgeBytes))
	sb.WriteString(fmt.Sprintf("  knowledge_bytes: %d\n", p.KnowledgeBytes))
	sb.WriteString(fmt.Sprintf("  after_knowledge_bytes: %d\n", p.AfterKnowledgeBytes))
	sb.WriteString(fmt.Sprintf("  input_resolution_bytes: %d\n", p.ResolutionBytes))
	sb.WriteString(fmt.Sprintf("  plan_bytes: %d\n", p.PlanBytes))
	sb.WriteString(fmt.Sprintf("  tools: %d\n", p.ToolCount))
	sb.WriteString(fmt.Sprintf("  sections: %d\n", p.SectionCount))
	sb.WriteString(fmt.Sprintf("  servers: %d, connectors: %d, learned_systems: %d, rag_sources: %d\n",
		p.ServerCount, p.ConnectorCount, p.LearnedSystemCount, p.RAGSourceCount))

	sb.WriteString("\nPrompt cache:\n")
	sb.WriteString(fmt.Sprintf("  contextual: entries=%d hits=%d misses=%d\n",
		cacheStats.ContextualEntries, cacheStats.ContextualHits, cacheStats.ContextualMisses))
	sb.WriteString(fmt.Sprintf("  minimal: entries=%d hits=%d misses=%d\n",
		cacheStats.MinimalEntries, cacheStats.MinimalHits, cacheStats.MinimalMisses))

	sb.WriteString("\nPrompt input cache:\n")
	sb.WriteString(fmt.Sprintf("  INFRACTL.md: hits=%d misses=%d\n", inputStats.InfractlMDHits, inputStats.InfractlMDMisses))
	sb.WriteString(fmt.Sprintf("  servers: hits=%d misses=%d\n", inputStats.ServersHits, inputStats.ServersMisses))
	sb.WriteString(fmt.Sprintf("  learned systems: hits=%d misses=%d\n", inputStats.LearnedSystemsHits, inputStats.LearnedSystemsMisses))
	sb.WriteString(fmt.Sprintf("  rag sources: hits=%d misses=%d\n", inputStats.RAGSourcesHits, inputStats.RAGSourcesMisses))
	sb.WriteString(fmt.Sprintf("  knowledge stats: hits=%d misses=%d\n", inputStats.KnowledgeStatsHits, inputStats.KnowledgeStatsMisses))

	return strings.TrimSpace(sb.String())
}

func countEnabledSections(sections SectionSet) int {
	total := 0
	for _, enabled := range sections {
		if enabled {
			total++
		}
	}
	return total
}
