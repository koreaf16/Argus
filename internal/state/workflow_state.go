package state

import (
	"strings"
	"time"
)

const metaWorkflowCard = "workflow_card"
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

// WorkflowCard 는 활성 다단계 작업의 메타데이터를 보관한다.
// 영속화는 별도로 하지 않으며 todostore 가 진척도(체크리스트)를 담당한다.
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

// WorkspaceRoleProfile binds an operational role to a concrete workspace and
// privilege context. It is deliberately non-secret; passwords remain in the
// workspace credential store.
//
// OSFamily / PackageManager / Architecture / OSVersion 는 discover phase 이후 채워지며,
// execute phase 에서 role-based intent 를 OS-specific 명령으로 번역할 때 사용된다.
// 이 필드들이 비어 있으면 LLM 이 plan 단계에서 sudo/dnf 같은 디테일을 미리 박지 않도록
// 가이드한다 (multi-session 가정 보호).
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

// SetWorkflowCard 는 활성 워크플로우 카드를 저장한다. nil 을 넘기면 클리어된다.
func (s *AppState) SetWorkflowCard(card *WorkflowCard) {
	cp := cloneWorkflowCard(card)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Metadata == nil {
		s.Metadata = make(map[string]interface{})
	}
	if cp == nil {
		delete(s.Metadata, metaWorkflowCard)
		clearAllWorkflowApprovalsLocked(s.Metadata)
		return
	}
	oldKey := workflowApprovalKey(workflowCardFromRaw(s.Metadata[metaWorkflowCard]))
	newKey := workflowApprovalKey(cp)
	if newKey != oldKey {
		clearAllWorkflowApprovalsLocked(s.Metadata)
	}
	s.Metadata[metaWorkflowCard] = cp
}

// WorkflowCard 는 현재 활성 카드를 반환한다. 없으면 nil.
func (s *AppState) WorkflowCard() *WorkflowCard {
	s.mu.RLock()
	if s.Metadata == nil {
		s.mu.RUnlock()
		return nil
	}
	raw := s.Metadata[metaWorkflowCard]
	s.mu.RUnlock()
	if raw == nil {
		return nil
	}

	return workflowCardFromRaw(raw)
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// SetWorkflowPhase 는 phase 만 갱신한다. 카드가 없으면 무시한다.
func (s *AppState) SetWorkflowPhase(phase string) {
	card := s.WorkflowCard()
	if card != nil {
		card.Phase = phase
		s.SetWorkflowCard(card)
	}
}

// ClearWorkflowCard 는 워크플로우 카드를 제거한다.
func (s *AppState) ClearWorkflowCard() {
	s.SetWorkflowCard(nil)
}

func (s *AppState) SetWorkflowApproved(approved bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Metadata == nil {
		s.Metadata = make(map[string]interface{})
	}
	key := workflowApprovalKey(workflowCardFromRaw(s.Metadata[metaWorkflowCard]))
	if key == "" {
		if approved {
			s.Metadata[metaLegacyUserApproved] = true
		} else {
			delete(s.Metadata, metaLegacyUserApproved)
		}
		return
	}
	if approved {
		s.Metadata[key] = true
		delete(s.Metadata, metaLegacyUserApproved)
		return
	}
	delete(s.Metadata, key)
	delete(s.Metadata, metaLegacyUserApproved)
}

func (s *AppState) WorkflowApproved() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return false
	}
	key := workflowApprovalKey(workflowCardFromRaw(s.Metadata[metaWorkflowCard]))
	if key == "" {
		v, _ := s.Metadata[metaLegacyUserApproved].(bool)
		return v
	}
	v, _ := s.Metadata[key].(bool)
	return v
}

func (s *AppState) ClearWorkflowApproval() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Metadata == nil {
		return
	}
	clearAllWorkflowApprovalsLocked(s.Metadata)
}

func cloneWorkflowCard(card *WorkflowCard) *WorkflowCard {
	if card == nil {
		return nil
	}
	cp := *card
	if card.Constraints != nil {
		cp.Constraints = append([]string(nil), card.Constraints...)
	}
	if card.WorkspaceRoles != nil {
		cp.WorkspaceRoles = make(map[string]string, len(card.WorkspaceRoles))
		for key, value := range card.WorkspaceRoles {
			cp.WorkspaceRoles[key] = value
		}
	}
	if card.WorkspaceRoleProfiles != nil {
		cp.WorkspaceRoleProfiles = make(map[string]WorkspaceRoleProfile, len(card.WorkspaceRoleProfiles))
		for key, value := range card.WorkspaceRoleProfiles {
			cp.WorkspaceRoleProfiles[key] = cloneWorkspaceRoleProfile(value)
		}
	}
	return &cp
}

func cloneWorkspaceRoleProfile(profile WorkspaceRoleProfile) WorkspaceRoleProfile {
	cp := profile
	if profile.AllowedTools != nil {
		cp.AllowedTools = append([]string(nil), profile.AllowedTools...)
	}
	return cp
}

func workflowCardFromRaw(raw interface{}) *WorkflowCard {
	switch v := raw.(type) {
	case nil:
		return nil
	case *WorkflowCard:
		return cloneWorkflowCard(v)
	case WorkflowCard:
		return cloneWorkflowCard(&v)
	case map[string]interface{}:
		card := &WorkflowCard{
			Title:    getString(v, "title"),
			Category: getString(v, "category"),
			Phase:    getString(v, "phase"),
			Goal:     getString(v, "goal"),
		}
		if sAt, ok := v["started_at"].(string); ok {
			card.StartedAt, _ = time.Parse(time.RFC3339, sAt)
		}
		switch c := v["constraints"].(type) {
		case []interface{}:
			for _, item := range c {
				if s, ok := item.(string); ok {
					card.Constraints = append(card.Constraints, s)
				}
			}
		case []string:
			card.Constraints = append([]string(nil), c...)
		}
		switch roles := v["workspace_roles"].(type) {
		case map[string]string:
			card.WorkspaceRoles = make(map[string]string, len(roles))
			for key, value := range roles {
				card.WorkspaceRoles[key] = value
			}
		case map[string]interface{}:
			card.WorkspaceRoles = make(map[string]string, len(roles))
			for key, rawVal := range roles {
				if val, ok := rawVal.(string); ok {
					card.WorkspaceRoles[key] = val
				}
			}
			if len(card.WorkspaceRoles) == 0 {
				card.WorkspaceRoles = nil
			}
		}
		switch profiles := v["workspace_role_profiles"].(type) {
		case map[string]WorkspaceRoleProfile:
			card.WorkspaceRoleProfiles = make(map[string]WorkspaceRoleProfile, len(profiles))
			for key, value := range profiles {
				card.WorkspaceRoleProfiles[key] = cloneWorkspaceRoleProfile(value)
			}
		case map[string]interface{}:
			card.WorkspaceRoleProfiles = parseWorkspaceRoleProfiles(profiles)
		}
		return card
	default:
		return nil
	}
}

func parseWorkspaceRoleProfiles(raw map[string]interface{}) map[string]WorkspaceRoleProfile {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]WorkspaceRoleProfile, len(raw))
	for key, value := range raw {
		profile := WorkspaceRoleProfile{}
		switch v := value.(type) {
		case WorkspaceRoleProfile:
			profile = cloneWorkspaceRoleProfile(v)
		case map[string]interface{}:
			profile = WorkspaceRoleProfile{
				Role:            getString(v, "role"),
				Channel:         getString(v, "channel"),
				Server:          getString(v, "server"),
				DefaultUser:     getString(v, "default_user"),
				AsUser:          getString(v, "as_user"),
				PrivilegeMethod: getString(v, "privilege_method"),
				MutationPolicy:  getString(v, "mutation_policy"),
				OSFamily:        getString(v, "os_family"),
				OSVersion:       getString(v, "os_version"),
				PackageManager:  getString(v, "package_manager"),
				Architecture:    getString(v, "architecture"),
			}
			profile.AllowedTools = getStringSlice(v, "allowed_tools")
		}
		if strings.TrimSpace(profile.Role) == "" {
			profile.Role = strings.TrimSpace(key)
		}
		if strings.TrimSpace(key) != "" {
			out[strings.TrimSpace(key)] = cloneWorkspaceRoleProfile(profile)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func getStringSlice(m map[string]interface{}, key string) []string {
	switch v := m[key].(type) {
	case []string:
		return append([]string(nil), v...)
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return nil
	}
}

func workflowApprovalKey(card *WorkflowCard) string {
	if card == nil {
		return ""
	}
	started := card.StartedAt.UTC().Format(time.RFC3339Nano)
	if card.StartedAt.IsZero() {
		started = "zero"
	}
	return metaWorkflowApprovalPrefix + started + ":" + strings.TrimSpace(card.Title)
}

func clearAllWorkflowApprovalsLocked(metadata map[string]interface{}) {
	delete(metadata, metaLegacyUserApproved)
	for key := range metadata {
		if strings.HasPrefix(key, metaWorkflowApprovalPrefix) {
			delete(metadata, key)
		}
	}
}

const metaPendingWorkflowInit = "pending_workflow_init"

// SetPendingWorkflowInit 는 다단계 작업 휴리스틱이 매치된 사용자 메시지가
// 들어왔음을 표시한다. 첫 도구 호출이 task_plan_init 이 아니면 차단된다.
func (s *AppState) SetPendingWorkflowInit(pending bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Metadata == nil {
		s.Metadata = make(map[string]interface{})
	}
	if pending {
		s.Metadata[metaPendingWorkflowInit] = true
		return
	}
	delete(s.Metadata, metaPendingWorkflowInit)
}

// PendingWorkflowInit 는 휴리스틱 플래그 상태를 반환한다.
func (s *AppState) PendingWorkflowInit() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return false
	}
	v, _ := s.Metadata[metaPendingWorkflowInit].(bool)
	return v
}
