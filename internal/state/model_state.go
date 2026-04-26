package state

import "github.com/koreaf16/argus/internal/types"

const (
	metaActiveModelAlias    = "active_model_alias"
	metaActiveModelName     = "active_model_name"
	metaActiveModelCtx      = "active_model_context"
	metaActiveModelProvider = "active_model_provider"
	metaContextUsedPercent  = "context_used_percent"
	metaEffortLevel         = "effort_level"
)

func (s *AppState) SetActiveModel(alias, display string, contextWin int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.metadata == nil {
		s.metadata = make(map[string]interface{})
	}
	s.metadata[metaActiveModelAlias] = alias
	s.metadata[metaActiveModelName] = display
	s.metadata[metaActiveModelCtx] = contextWin
}

func (s *AppState) SetActiveModelProvider(provider string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.metadata == nil {
		s.metadata = make(map[string]interface{})
	}
	s.metadata[metaActiveModelProvider] = provider
}

func (s *AppState) ActiveModelProvider() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.metadata == nil {
		return ""
	}
	v, _ := s.metadata[metaActiveModelProvider].(string)
	return v
}

func (s *AppState) ActiveModel() (alias, display string, contextWin int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.metadata == nil {
		return "", "", 0
	}
	a, _ := s.metadata[metaActiveModelAlias].(string)
	d, _ := s.metadata[metaActiveModelName].(string)
	c, _ := s.metadata[metaActiveModelCtx].(int)
	return a, d, c
}

func (s *AppState) SetEffortLevel(level string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.metadata == nil {
		s.metadata = make(map[string]interface{})
	}
	s.metadata[metaEffortLevel] = level
}

func (s *AppState) GetEffortLevel() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.metadata == nil {
		return "high"
	}
	v, _ := s.metadata[metaEffortLevel].(string)
	if v == "" {
		return "high"
	}
	return v
}

func (s *AppState) ActiveModelContext() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.metadata == nil {
		return 0
	}
	c, _ := s.metadata[metaActiveModelCtx].(int)
	return c
}

func (s *AppState) SetContextUsedPercent(percent int) {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.metadata == nil {
		s.metadata = make(map[string]interface{})
	}
	s.metadata[metaContextUsedPercent] = percent
}

func (s *AppState) ContextUsedPercent() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.metadata == nil {
		return 0
	}
	p, _ := s.metadata[metaContextUsedPercent].(int)
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func (s *AppState) SetPermissionMode(mode types.PermissionMode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Permissions = string(mode)
}

func (s *AppState) GetPermissionMode() types.PermissionMode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Permissions == "" {
		return types.PermissionModeDefault
	}
	return types.PermissionMode(s.Permissions)
}

func (s *AppState) InPlanMode() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Mode == "plan" || types.PermissionMode(s.Permissions) == types.PermissionModePlan
}
