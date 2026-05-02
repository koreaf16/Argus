package tools

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/koreaf16/argus/internal/services/llm"
)

type Registry struct {
	mu     sync.RWMutex
	tools  map[string]Tool
	source map[string]string
	alias  map[string]string
}

func NewRegistry() *Registry {
	return &Registry{
		tools:  make(map[string]Tool),
		source: make(map[string]string),
		alias:  make(map[string]string),
	}
}

func (r *Registry) Register(t Tool) error {
	return r.RegisterFromSource("", t)
}

func (r *Registry) RegisterFromSource(source string, t Tool) error {
	if t == nil {
		return fmt.Errorf("tool is nil")
	}
	name := t.Name()
	if name == "" {
		return fmt.Errorf("tool name is empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool already registered: %s", name)
	}
	norm := normalizeToolName(name)
	if existing, exists := r.alias[norm]; exists && !strings.EqualFold(existing, name) {
		return fmt.Errorf("tool name conflicts with existing alias: %s", name)
	}
	r.tools[name] = t
	r.source[name] = source
	r.alias[norm] = name
	return nil
}

func (r *Registry) RegisterFromMCP(server string, t Tool) error {
	return r.RegisterFromSource("mcp:"+server, t)
}

func (r *Registry) RegisterAlias(alias string, target string) error {
	a := strings.TrimSpace(alias)
	t := strings.TrimSpace(target)
	if a == "" || t == "" {
		return fmt.Errorf("alias and target are required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.tools[t]; !ok {
		return fmt.Errorf("target tool not found: %s", t)
	}

	normAlias := normalizeToolName(a)
	if existing, ok := r.alias[normAlias]; ok && existing != t {
		return fmt.Errorf("alias already mapped: %s -> %s", a, existing)
	}
	r.alias[normAlias] = t
	return nil
}

func (r *Registry) UnregisterSource(source string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	removed := 0
	for name, src := range r.source {
		if src != source {
			continue
		}
		delete(r.tools, name)
		delete(r.source, name)
		r.removeAliasesForLocked(name)
		removed++
	}
	return removed
}

func (r *Registry) UnregisterServer(server string) int {
	return r.UnregisterSource("mcp:" + server)
}

func (r *Registry) ListBySource(source string) []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		if r.source[name] != source {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Tool, 0, len(names))
	for _, name := range names {
		out = append(out, r.tools[name])
	}
	return out
}

func (r *Registry) ListByServer(server string) []Tool {
	return r.ListBySource("mcp:" + server)
}

func (r *Registry) Lookup(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	target, ok := r.alias[normalizeToolName(name)]
	if !ok {
		return nil, false
	}
	t, ok := r.tools[target]
	return t, ok
}

func (r *Registry) Unregister(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	target, ok := r.alias[normalizeToolName(name)]
	if !ok {
		return false
	}
	if _, ok := r.tools[target]; !ok {
		return false
	}
	delete(r.tools, target)
	delete(r.source, target)
	r.removeAliasesForLocked(target)
	return true
}

func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Tool, 0, len(names))
	for _, name := range names {
		out = append(out, r.tools[name])
	}
	return out
}

func (r *Registry) ToolSpecs(ctx Context) []llm.ToolSpec {
	tools := r.List()
	specs := make([]llm.ToolSpec, 0, len(tools))
	for _, t := range tools {
		// 가시성 필터링 적용
		if v, ok := t.(VisibleFilter); ok {
			if !v.IsVisible(ctx) {
				continue
			}
		}

		specs = append(specs, llm.ToolSpec{
			Name:        t.Name(),
			Description: t.Description(ctx),
			InputSchema: t.InputSchema(),
		})
	}
	return specs
}

func (r *Registry) removeAliasesForLocked(target string) {
	for alias, name := range r.alias {
		if name == target {
			delete(r.alias, alias)
		}
	}
}

func normalizeToolName(name string) string {
	return CanonicalName(name)
}
