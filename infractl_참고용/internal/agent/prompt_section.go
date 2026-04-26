// Package agent
// File: prompt_section.go
// Description: 시스템 프롬프트 섹션 타입 정의
// Responsibility: PromptSection 상수와 SectionSet 집합 타입 제공

package agent

// PromptSection은 시스템 프롬프트의 개별 섹션을 식별한다.
type PromptSection string

const (
	SectionCore           PromptSection = "core"
	SectionEnvironment    PromptSection = "environment"
	SectionTools          PromptSection = "tools"
	SectionServers        PromptSection = "servers"
	SectionServerFocus    PromptSection = "server_focus"
	SectionGuardrails     PromptSection = "guardrails"
	SectionBehavior       PromptSection = "behavior"
	SectionToolPriority   PromptSection = "tool_priority"
	SectionToolSelection  PromptSection = "tool_selection"
	SectionErrorRecovery  PromptSection = "error_recovery"
	SectionDiscovery      PromptSection = "discovery"
	SectionTaskCompletion PromptSection = "task_completion"
	SectionSafety         PromptSection = "safety"
	SectionGrounding      PromptSection = "grounding"
	SectionPorts          PromptSection = "ports"
	SectionPhasePlanning  PromptSection = "phase_planning"
	SectionRAG            PromptSection = "rag"
	SectionLearnedSystems PromptSection = "learned_systems"
	SectionConnectors     PromptSection = "connectors"
	SectionConnectorFirst PromptSection = "connector_first"
	SectionInfractlMD     PromptSection = "infractl_md"
)

// SectionSet은 포함할 프롬프트 섹션의 집합이다.
type SectionSet map[PromptSection]bool

// Has는 주어진 섹션이 집합에 포함되어 있는지 반환한다.
func (s SectionSet) Has(section PromptSection) bool {
	return s[section]
}
