package query

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/koreaf16/argus/internal/state"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
	permutils "github.com/koreaf16/argus/internal/utils/permissions"
)

func (e *Engine) SetApprovalGate(gate ApprovalGate) {
	e.mu.Lock()
	e.deps.ApproveTool = gate
	e.mu.Unlock()
}

func (e *Engine) InvalidatePermissionRuleCache() {
	e.mu.Lock()
	e.permissionRulesCache = nil
	e.permissionRulesCacheValid = false
	e.permissionRulesCachedAt = time.Time{}
	e.mu.Unlock()
}

func (e *Engine) loadDiskPermissionRulesCached() []types.PermissionRule {
	now := time.Now()
	e.mu.RLock()
	cacheValid := e.permissionRulesCacheValid
	cachedAt := e.permissionRulesCachedAt
	cached := clonePermissionRules(e.permissionRulesCache)
	e.mu.RUnlock()
	if cacheValid && now.Sub(cachedAt) < permissionRuleCacheTTL {
		return cached
	}
	rules := permutils.LoadAllPermissionRulesFromDisk()
	e.mu.Lock()
	e.permissionRulesCache = clonePermissionRules(rules)
	e.permissionRulesCachedAt = now
	e.permissionRulesCacheValid = true
	e.mu.Unlock()
	return rules
}

func (e *Engine) findPreApprovedRule(appState *state.AppState, toolName string, input json.RawMessage) *types.PermissionRule {
	rules := e.loadDiskPermissionRulesCached()
	if appState != nil {
		rules = append(rules, appState.SessionPermissionRules()...)
	}
	if len(rules) == 0 {
		return nil
	}

	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return nil
	}
	command := strings.TrimSpace(tool.ExtractStringInput(input, "command"))
	isShellToolName := strings.EqualFold(toolName, "bash") || strings.EqualFold(toolName, "powershell")
	caseInsensitiveShell := strings.EqualFold(toolName, "powershell")

	for _, rule := range rules {
		if rule.RuleBehavior != types.BehaviorAllow {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(rule.RuleValue.ToolName), toolName) {
			continue
		}
		content := strings.TrimSpace(rule.RuleValue.RuleContent)
		if content == "" {
			matched := rule
			return &matched
		}
		if !isShellToolName || command == "" {
			continue
		}
		scopedContent, ok := matchScopedShellRule(content, input)
		if !ok {
			continue
		}
		content = scopedContent
		parsed := permutils.ParsePermissionRule(content)
		if permutils.MatchesPermissionRule(parsed, command, caseInsensitiveShell) {
			matched := rule
			return &matched
		}
	}
	return nil
}

func matchScopedShellRule(content string, input json.RawMessage) (string, bool) {
	if !strings.HasPrefix(content, "ctx[") {
		if hasExplicitExecutionScope(input) {
			return "", false
		}
		return content, true
	}
	end := strings.Index(content, "]:")
	if end < 0 {
		return "", false
	}
	scopeText := content[len("ctx["):end]
	rest := strings.TrimSpace(content[end+2:])
	if rest == "" {
		return "", false
	}
	var obj map[string]any
	if err := json.Unmarshal(input, &obj); err != nil {
		return "", false
	}
	for _, pair := range strings.Split(scopeText, ";") {
		key, want, ok := strings.Cut(pair, "=")
		if !ok {
			return "", false
		}
		got, _ := obj[strings.TrimSpace(key)].(string)
		if !strings.EqualFold(strings.TrimSpace(got), strings.TrimSpace(want)) {
			return "", false
		}
	}
	return rest, true
}

func hasExplicitExecutionScope(input json.RawMessage) bool {
	var obj map[string]any
	if err := json.Unmarshal(input, &obj); err != nil {
		return false
	}
	for _, key := range []string{"server", "role", "channel", "as_user", "privilege_method"} {
		if value, ok := obj[key].(string); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func clonePermissionRules(in []types.PermissionRule) []types.PermissionRule {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.PermissionRule, len(in))
	copy(out, in)
	return out
}
