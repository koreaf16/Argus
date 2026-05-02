package state

import "github.com/koreaf16/argus/internal/types"

const (
	metaPrePlanMode            = "pre_plan_mode"
	metaExtraDirs              = "additional_working_directories"
	metaSessionPermissionRules = "session_permission_rules"
)

func (s *AppState) SetPrePlanMode(mode types.PermissionMode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Metadata == nil {
		s.Metadata = make(map[string]interface{})
	}
	s.Metadata[metaPrePlanMode] = string(mode)
}

func (s *AppState) PrePlanMode() types.PermissionMode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return types.PermissionModeDefault
	}
	v, _ := s.Metadata[metaPrePlanMode].(string)
	if v == "" {
		return types.PermissionModeDefault
	}
	return types.PermissionMode(v)
}

func (s *AppState) ClearPrePlanMode() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Metadata == nil {
		return
	}
	delete(s.Metadata, metaPrePlanMode)
}

func (s *AppState) SetAdditionalWorkingDirectories(dirs []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Metadata == nil {
		s.Metadata = make(map[string]interface{})
	}
	cp := make([]string, len(dirs))
	copy(cp, dirs)
	s.Metadata[metaExtraDirs] = cp
}

func (s *AppState) AdditionalWorkingDirectories() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return nil
	}
	src, _ := s.Metadata[metaExtraDirs].([]string)
	out := make([]string, len(src))
	copy(out, src)
	return out
}

func (s *AppState) AddSessionPermissionRule(rule types.PermissionRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Metadata == nil {
		s.Metadata = make(map[string]interface{})
	}
	if rule.Source == "" {
		rule.Source = types.RuleSourceSession
	}
	existing, _ := s.Metadata[metaSessionPermissionRules].([]types.PermissionRule)
	for _, it := range existing {
		if it.RuleBehavior == rule.RuleBehavior &&
			it.Source == rule.Source &&
			it.RuleValue.ToolName == rule.RuleValue.ToolName &&
			it.RuleValue.RuleContent == rule.RuleValue.RuleContent {
			return
		}
	}
	next := append(existing, rule)
	s.Metadata[metaSessionPermissionRules] = next
}

func (s *AppState) SessionPermissionRules() []types.PermissionRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return nil
	}
	existing, _ := s.Metadata[metaSessionPermissionRules].([]types.PermissionRule)
	out := make([]types.PermissionRule, len(existing))
	copy(out, existing)
	return out
}

func (s *AppState) ClearSessionPermissionRules() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Metadata == nil {
		return
	}
	delete(s.Metadata, metaSessionPermissionRules)
}
