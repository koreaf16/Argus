package state

import "strconv"

const (
	metaSessionID       = "session_id"
	metaActiveMCP       = "active_mcp_servers"
	metaActiveSkillList = "active_skills"
	metaActiveWorkspace = "active_workspace"
	metaEphemeralServers = "ephemeral_servers"
)

func (s *AppState) SetEphemeralServers(servers []interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Metadata == nil {
		s.Metadata = make(map[string]interface{})
	}
	s.Metadata[metaEphemeralServers] = servers
}

func (s *AppState) EphemeralServers() []interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return nil
	}
	v, _ := s.Metadata[metaEphemeralServers].([]interface{})
	return v
}

func (s *AppState) SetSessionID(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Metadata == nil {
		s.Metadata = make(map[string]interface{})
	}
	s.Metadata[metaSessionID] = id
}

func (s *AppState) SessionID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return ""
	}
	v, _ := s.Metadata[metaSessionID].(string)
	return v
}

func (s *AppState) SetActiveMCPServers(servers []string) {
	s.mu.Lock()
	if s.Metadata == nil {
		s.Metadata = make(map[string]interface{})
	}
	beforeSlice, _ := s.Metadata[metaActiveMCP].([]string)
	beforeCount := len(beforeSlice)
	cp := make([]string, len(servers))
	copy(cp, servers)
	s.Metadata[metaActiveMCP] = cp
	hook := s.onStateChange
	s.mu.Unlock()
	if hook != nil && beforeCount != len(servers) {
		hook("mcp_server_count", strconv.Itoa(beforeCount), strconv.Itoa(len(servers)))
	}
}

func (s *AppState) ActiveMCPServers() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return nil
	}
	src, _ := s.Metadata[metaActiveMCP].([]string)
	cp := make([]string, len(src))
	copy(cp, src)
	return cp
}

func (s *AppState) SetActiveSkills(skills []string) {
	s.mu.Lock()
	if s.Metadata == nil {
		s.Metadata = make(map[string]interface{})
	}
	beforeSlice, _ := s.Metadata[metaActiveSkillList].([]string)
	beforeCount := len(beforeSlice)
	cp := make([]string, len(skills))
	copy(cp, skills)
	s.Metadata[metaActiveSkillList] = cp
	hook := s.onStateChange
	s.mu.Unlock()
	if hook != nil && beforeCount != len(skills) {
		hook("skill_count", strconv.Itoa(beforeCount), strconv.Itoa(len(skills)))
	}
}

func (s *AppState) ActiveSkills() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return nil
	}
	src, _ := s.Metadata[metaActiveSkillList].([]string)
	cp := make([]string, len(src))
	copy(cp, src)
	return cp
}

func (s *AppState) SetActiveWorkspace(alias string) {
	s.mu.Lock()
	if s.Metadata == nil {
		s.Metadata = make(map[string]interface{})
	}
	before, _ := s.Metadata[metaActiveWorkspace].(string)
	s.Metadata[metaActiveWorkspace] = alias
	hook := s.onStateChange
	s.mu.Unlock()
	if hook != nil && before != alias {
		hook("workspace", before, alias)
	}
}

func (s *AppState) ActiveWorkspace() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return ""
	}
	v, _ := s.Metadata[metaActiveWorkspace].(string)
	return v
}
