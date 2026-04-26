// Package ultraplan — Ultra-plan mode coordination (Phase 1 stub).
//
// 파일 역할: Provides plan mode execution coordination and management.
//
//	For Phase 2: Will include full plan execution orchestration.
//
// 포함 모듈:
//   - CCRSession: Coordinator for plan execution
//
// 호출/사용 방식:
//   - Plan execution engines use this for task coordination
//
// 연결:
//   - internal/planmode: planmode.go
//   - 원본: src/utils/planModeV2.ts (Phase 2 extension)
package ultraplan

import (
	"time"
)

// CCRSession represents a plan coordination session.
type CCRSession struct {
	ID        string
	StartedAt time.Time
	Tasks     map[string]interface{}
}

// NewCCRSession creates a new coordination session.
func NewCCRSession(id string) *CCRSession {
	return &CCRSession{
		ID:        id,
		StartedAt: time.Now(),
		Tasks:     make(map[string]interface{}),
	}
}

// AddTask adds a task to the session.
func (s *CCRSession) AddTask(taskID string, task interface{}) {
	s.Tasks[taskID] = task
}

// GetTask retrieves a task from the session.
func (s *CCRSession) GetTask(taskID string) (interface{}, bool) {
	task, ok := s.Tasks[taskID]
	return task, ok
}

// RemoveTask removes a task from the session.
func (s *CCRSession) RemoveTask(taskID string) {
	delete(s.Tasks, taskID)
}

// GetElapsedTime returns the elapsed time since session started.
func (s *CCRSession) GetElapsedTime() time.Duration {
	return time.Since(s.StartedAt)
}
