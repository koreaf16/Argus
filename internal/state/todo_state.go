package state

import "github.com/koreaf16/argus/internal/types"

const metaTodos = "todos_by_session"

func (s *AppState) SetTodos(sessionID string, todos []types.TodoItem) {
	if sessionID == "" {
		sessionID = "default"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.metadata == nil {
		s.metadata = make(map[string]interface{})
	}
	m, _ := s.metadata[metaTodos].(map[string][]types.TodoItem)
	if m == nil {
		m = make(map[string][]types.TodoItem)
	}
	cp := make([]types.TodoItem, len(todos))
	copy(cp, todos)
	m[sessionID] = cp
	s.metadata[metaTodos] = m
}

func (s *AppState) Todos(sessionID string) []types.TodoItem {
	if sessionID == "" {
		sessionID = "default"
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.metadata == nil {
		return nil
	}
	m, _ := s.metadata[metaTodos].(map[string][]types.TodoItem)
	if m == nil {
		return nil
	}
	src := m[sessionID]
	out := make([]types.TodoItem, len(src))
	copy(out, src)
	return out
}
