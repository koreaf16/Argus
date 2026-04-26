package state

const (
	metaSessionID       = "session_id"
	metaActiveMCP       = "active_mcp_servers"
	metaActiveSkillList = "active_skills"
	metaActiveWorkspace = "active_workspace"
)

func (s *AppState) SetSessionID(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.metadata == nil {
		s.metadata = make(map[string]interface{})
	}
	s.metadata[metaSessionID] = id
}

func (s *AppState) SessionID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.metadata == nil {
		return ""
	}
	v, _ := s.metadata[metaSessionID].(string)
	return v
}

func (s *AppState) SetActiveMCPServers(servers []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.metadata == nil {
		s.metadata = make(map[string]interface{})
	}
	cp := make([]string, len(servers))
	copy(cp, servers)
	s.metadata[metaActiveMCP] = cp
}

func (s *AppState) ActiveMCPServers() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.metadata == nil {
		return nil
	}
	src, _ := s.metadata[metaActiveMCP].([]string)
	cp := make([]string, len(src))
	copy(cp, src)
	return cp
}

func (s *AppState) SetActiveSkills(skills []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.metadata == nil {
		s.metadata = make(map[string]interface{})
	}
	cp := make([]string, len(skills))
	copy(cp, skills)
	s.metadata[metaActiveSkillList] = cp
}

func (s *AppState) ActiveSkills() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.metadata == nil {
		return nil
	}
	src, _ := s.metadata[metaActiveSkillList].([]string)
	cp := make([]string, len(src))
	copy(cp, src)
	return cp
}

func (s *AppState) SetActiveWorkspace(alias string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.metadata == nil {
		s.metadata = make(map[string]interface{})
	}
	s.metadata[metaActiveWorkspace] = alias
}

func (s *AppState) ActiveWorkspace() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.metadata == nil {
		return ""
	}
	v, _ := s.metadata[metaActiveWorkspace].(string)
	return v
}
