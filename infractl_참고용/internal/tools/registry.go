// Package tools
// File: registry.go
// Description: 도구 등록, 조회, LLM용 JSON Schema 변환을 담당하는 레지스트리
// Responsibility: 도구 생명주기 관리 및 LLM 도구 정의 변환

package tools

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yourorg/infractl/internal/llm"
)

// Registry는 등록된 도구들을 관리하는 레지스트리이다.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry는 빈 도구 레지스트리를 생성한다.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register는 도구를 레지스트리에 등록한다.
// 같은 이름의 도구가 이미 등록되어 있으면 error를 반환한다.
func (r *Registry) Register(tool Tool) error {
	name := tool.Name()
	key := canonicalToolKey(name)
	if _, exists := r.tools[key]; exists {
		return fmt.Errorf("tool already registered: %s", name)
	}
	r.tools[key] = tool
	return nil
}

// Unregister는 이름으로 도구를 레지스트리에서 제거한다.
// 커넥터 비활성화 시 해당 도구 세트를 제거하는 데 사용한다.
func (r *Registry) Unregister(name string) {
	delete(r.tools, canonicalToolKey(name))
}

// Has는 이름으로 도구가 등록되어 있는지 확인한다.
func (r *Registry) Has(name string) bool {
	_, ok := r.tools[canonicalToolKey(name)]
	return ok
}

// Get은 이름으로 도구를 조회한다.
func (r *Registry) Get(name string) (Tool, bool) {
	tool, ok := r.tools[canonicalToolKey(name)]
	return tool, ok
}

// List는 등록된 모든 도구를 이름순으로 정렬하여 반환한다.
func (r *Registry) List() []Tool {
	result := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name() < result[j].Name()
	})
	return result
}

// GetEnabled는 IsEnabled()=true인 도구만 반환한다.
// LLM에 전달할 도구 목록을 구성할 때 사용한다.
func (r *Registry) GetEnabled() []Tool {
	all := r.List()
	result := make([]Tool, 0, len(all))
	for _, t := range all {
		if t.IsEnabled() {
			result = append(result, t)
		}
	}
	return result
}

// ToToolDefs는 활성화된 도구를 OpenAI function calling 형식으로 변환한다.
// LLM API 요청 시 tools 필드에 사용된다.
func (r *Registry) ToToolDefs() []llm.ToolDef {
	tools := r.GetEnabled()
	defs := make([]llm.ToolDef, len(tools))
	for i, t := range tools {
		defs[i] = llm.ToolDef{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        t.Name(),
				Description: modelPromptForTool(t),
				Parameters:  t.Parameters(),
			},
		}
	}
	return defs
}

// ToToolDefsFiltered는 allowed에 포함된 이름의 도구만 ToolDef로 변환한다.
// chat 의도 등 최소 도구만 필요한 경우 페이로드를 줄이기 위해 사용한다.
func (r *Registry) ToToolDefsFiltered(allowed map[string]bool) []llm.ToolDef {
	tools := r.GetEnabled()
	normalizedAllowed := normalizeToolAllowList(allowed)
	defs := make([]llm.ToolDef, 0, len(allowed))
	for _, t := range tools {
		if !normalizedAllowed[canonicalToolKey(t.Name())] {
			continue
		}
		defs = append(defs, llm.ToolDef{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        t.Name(),
				Description: modelPromptForTool(t),
				Parameters:  t.Parameters(),
			},
		})
	}
	return defs
}

func modelPromptForTool(t Tool) string {
	if mp, ok := t.(ModelPromptDescriber); ok {
		if v := mp.ModelPrompt(); v != "" {
			return v
		}
	}
	return t.Description()
}

// GetEnabledFiltered는 allowed에 포함된 이름의 활성 도구만 반환한다.
func (r *Registry) GetEnabledFiltered(allowed map[string]bool) []Tool {
	all := r.GetEnabled()
	normalizedAllowed := normalizeToolAllowList(allowed)
	result := make([]Tool, 0, len(allowed))
	for _, t := range all {
		if normalizedAllowed[canonicalToolKey(t.Name())] {
			result = append(result, t)
		}
	}
	return result
}

func canonicalToolKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func normalizeToolAllowList(allowed map[string]bool) map[string]bool {
	if len(allowed) == 0 {
		return map[string]bool{}
	}
	normalized := make(map[string]bool, len(allowed))
	for name, ok := range allowed {
		normalized[canonicalToolKey(name)] = ok
	}
	return normalized
}
