package state

import "github.com/koreaf16/argus/internal/types"

const metaTodos = "todos_by_session"

func (s *AppState) SetTodos(sessionID string, todos []types.TodoItem) {
	if sessionID == "" {
		sessionID = "default"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Metadata == nil {
		s.Metadata = make(map[string]interface{})
	}
	// JSON 역직렬화 시 map[string]interface{} 로 들어올 수 있으므로 안전하게 처리
	var m map[string][]types.TodoItem
	if raw, ok := s.Metadata[metaTodos]; ok {
		m, _ = raw.(map[string][]types.TodoItem)
	}
	if m == nil {
		m = make(map[string][]types.TodoItem)
	}
	cp := make([]types.TodoItem, len(todos))
	copy(cp, todos)
	m[sessionID] = cp
	s.Metadata[metaTodos] = m
}

func (s *AppState) Todos(sessionID string) []types.TodoItem {
	if sessionID == "" {
		sessionID = "default"
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return nil
	}
	raw := s.Metadata[metaTodos]
	if raw == nil {
		return nil
	}

	// [보안] JSON 역직렬화 결과(map[string]interface{}) 대응
	if m, ok := raw.(map[string]interface{}); ok {
		// 현재 세션에 대한 데이터만 추출하여 반환 (필요시 전체 변환 로직 추가 가능)
		if sessionData, ok := m[sessionID].([]interface{}); ok {
			out := make([]types.TodoItem, 0, len(sessionData))
			for _, item := range sessionData {
				if im, ok := item.(map[string]interface{}); ok {
					todo := types.TodoItem{
						Content:    getString(im, "content"),
						Status:     types.TodoStatus(getString(im, "status")),
						ActiveForm: getString(im, "activeForm"),
					}
					out = append(out, todo)
				}
			}
			return out
		}
	}

	m, _ := raw.(map[string][]types.TodoItem)
	if m == nil {
		return nil
	}
	src := m[sessionID]
	out := make([]types.TodoItem, len(src))
	copy(out, src)
	return out
}
