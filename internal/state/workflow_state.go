package state

import (
	"strings"
	"time"
)

const metaLegacyUserApproved = "user_approved"
const metaWorkflowApprovalPrefix = "workflow_approved:"

// Phase 상수 — 6단계 표준 워크플로우.
const (
	WorkflowPhaseDiscover  = "discover"
	WorkflowPhaseResearch  = "research"
	WorkflowPhaseInterview = "interview"
	WorkflowPhasePlan      = "plan"
	WorkflowPhaseExecute   = "execute"
	WorkflowPhaseVerify    = "verify"
	WorkflowPhaseDone      = "done"
)

// Category 상수 — task_plan_init 분류.
const (
	WorkflowCategoryInstall     = "install"
	WorkflowCategoryMigration   = "migration"
	WorkflowCategoryTuning      = "tuning"
	WorkflowCategoryIntegration = "integration"
	WorkflowCategoryOther       = "other"
)

// WorkspaceRoleProfile binds an operational role to a concrete workspace and
// privilege context. It is deliberately non-secret; passwords remain in the
// workspace credential store.
type WorkspaceRoleProfile struct {
	Role            string   `json:"role,omitempty"`
	Channel         string   `json:"channel,omitempty"`
	Server          string   `json:"server,omitempty"`
	DefaultUser     string   `json:"default_user,omitempty"`
	AsUser          string   `json:"as_user,omitempty"`
	PrivilegeMethod string   `json:"privilege_method,omitempty"`
	AllowedTools    []string `json:"allowed_tools,omitempty"`
	MutationPolicy  string   `json:"mutation_policy,omitempty"`
	OSFamily        string   `json:"os_family,omitempty"`
	OSVersion       string   `json:"os_version,omitempty"`
	PackageManager  string   `json:"package_manager,omitempty"`
	Architecture    string   `json:"architecture,omitempty"`
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func (s *AppState) SetWorkflowApproved(approved bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Metadata == nil {
		s.Metadata = make(map[string]interface{})
	}
	if approved {
		s.Metadata[metaLegacyUserApproved] = true
	} else {
		delete(s.Metadata, metaLegacyUserApproved)
	}
}

func (s *AppState) WorkflowApproved() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return false
	}
	v, _ := s.Metadata[metaLegacyUserApproved].(bool)
	return v
}

func (s *AppState) ClearWorkflowApproval() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Metadata == nil {
		return
	}
	delete(s.Metadata, metaLegacyUserApproved)
	for key := range s.Metadata {
		if strings.HasPrefix(key, metaWorkflowApprovalPrefix) {
			delete(s.Metadata, key)
		}
	}
}

// WorkflowCard dummy structure for backward compatibility
type WorkflowCard struct {
	Title                 string                          `json:"title"`
	Category              string                          `json:"category"`
	Phase                 string                          `json:"phase"`
	Goal                  string                          `json:"goal,omitempty"`
	Constraints           []string                        `json:"constraints,omitempty"`
	WorkspaceRoles        map[string]string               `json:"workspace_roles,omitempty"`
	WorkspaceRoleProfiles map[string]WorkspaceRoleProfile `json:"workspace_role_profiles,omitempty"`
	StartedAt             time.Time                       `json:"started_at"`
}

func (s *AppState) WorkflowCard() *WorkflowCard {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneWorkflowCard(s.workflowCard)
}

func (s *AppState) SetWorkflowCard(c *WorkflowCard) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workflowCard = cloneWorkflowCard(c)
}

func (s *AppState) SetWorkflowPhase(phase string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.workflowCard != nil {
		s.workflowCard.Phase = phase
	}
}

func (s *AppState) ClearWorkflowCard() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workflowCard = nil
}

func (s *AppState) SetPendingWorkflowInit(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingWorkflowInit = v
}

func (s *AppState) PendingWorkflowInit() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pendingWorkflowInit
}

func cloneWorkflowCard(c *WorkflowCard) *WorkflowCard {
	if c == nil {
		return nil
	}
	clone := *c
	if len(c.Constraints) > 0 {
		clone.Constraints = make([]string, len(c.Constraints))
		copy(clone.Constraints, c.Constraints)
	}
	if len(c.WorkspaceRoles) > 0 {
		clone.WorkspaceRoles = make(map[string]string, len(c.WorkspaceRoles))
		for k, v := range c.WorkspaceRoles {
			clone.WorkspaceRoles[k] = v
		}
	}
	if len(c.WorkspaceRoleProfiles) > 0 {
		clone.WorkspaceRoleProfiles = make(map[string]WorkspaceRoleProfile, len(c.WorkspaceRoleProfiles))
		for k, v := range c.WorkspaceRoleProfiles {
			clone.WorkspaceRoleProfiles[k] = v
		}
	}
	return &clone
}
