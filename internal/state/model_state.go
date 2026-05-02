package state

import (
	"strconv"

	"github.com/koreaf16/argus/internal/types"
)

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
	if s.Metadata == nil {
		s.Metadata = make(map[string]interface{})
	}
	before, _ := s.Metadata[metaActiveModelAlias].(string)
	s.Metadata[metaActiveModelAlias] = alias
	s.Metadata[metaActiveModelName] = display
	s.Metadata[metaActiveModelCtx] = contextWin
	hook := s.onStateChange
	s.mu.Unlock()
	if hook != nil && before != alias {
		hook("active_model", before, alias)
	}
}

func (s *AppState) SetActiveModelProvider(provider string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Metadata == nil {
		s.Metadata = make(map[string]interface{})
	}
	s.Metadata[metaActiveModelProvider] = provider
}

func (s *AppState) ActiveModelProvider() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return ""
	}
	v, _ := s.Metadata[metaActiveModelProvider].(string)
	return v
}

func (s *AppState) ActiveModel() (alias, display string, contextWin int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return "", "", 0
	}
	a, _ := s.Metadata[metaActiveModelAlias].(string)
	d, _ := s.Metadata[metaActiveModelName].(string)
	c, _ := s.Metadata[metaActiveModelCtx].(int)
	return a, d, c
}

func (s *AppState) SetEffortLevel(level string) {
	s.mu.Lock()
	if s.Metadata == nil {
		s.Metadata = make(map[string]interface{})
	}
	before, _ := s.Metadata[metaEffortLevel].(string)
	s.Metadata[metaEffortLevel] = level
	hook := s.onStateChange
	s.mu.Unlock()
	if hook != nil && before != level {
		hook("effort_level", before, level)
	}
}

func (s *AppState) GetEffortLevel() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return "high"
	}
	v, _ := s.Metadata[metaEffortLevel].(string)
	if v == "" {
		return "high"
	}
	return v
}

func (s *AppState) ActiveModelContext() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return 0
	}
	c, _ := s.Metadata[metaActiveModelCtx].(int)
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
	if s.Metadata == nil {
		s.Metadata = make(map[string]interface{})
	}
	before, _ := s.Metadata[metaContextUsedPercent].(int)
	s.Metadata[metaContextUsedPercent] = percent
	hook := s.onStateChange
	s.mu.Unlock()
	if hook != nil && before != percent {
		hook("context_used_percent", strconv.Itoa(before), strconv.Itoa(percent))
	}
}

func (s *AppState) ContextUsedPercent() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return 0
	}
	p, _ := s.Metadata[metaContextUsedPercent].(int)
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
	before := s.Permissions
	s.Permissions = string(mode)
	hook := s.onStateChange
	s.mu.Unlock()
	if hook != nil && before != string(mode) {
		hook("permission_mode", before, string(mode))
	}
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

func (s *AppState) InYoloMode() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return types.PermissionMode(s.Permissions) == types.PermissionModeBypassPermissions
}
