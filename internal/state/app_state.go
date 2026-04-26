// Package state — Application state management.
//
// 파일 역할: Defines AppState struct that holds all application state including
//
//	messages, mode, and permissions.
//
// 포함 모듈:
//   - AppState: Main state struct
//
// 호출/사용 방식:
//   - Internal state management
//
// 연결:
//   - internal/state
//   - 원본: src/state/ state types
package state

import (
	"sync"
	"time"
)

// AppState represents the entire application state.
type AppState struct {
	mu          sync.RWMutex
	Messages    []Message
	Mode        string // "normal", "plan"
	Permissions string // "ask", "allow", "deny"
	metadata    map[string]interface{}
}

// Message represents a single message in the conversation.
type Message struct {
	ID        string
	Role      string
	Content   interface{}
	Timestamp time.Time
}

// NewAppState creates a new application state.
func NewAppState() *AppState {
	return &AppState{
		Messages:    make([]Message, 0),
		Mode:        "normal",
		Permissions: "ask",
		metadata:    make(map[string]interface{}),
	}
}

// AddMessage adds a message to the state.
func (s *AppState) AddMessage(message Message) {
	s.mu.Lock()
	defer s.mu.Unlock()

	message.Timestamp = time.Now()
	s.Messages = append(s.Messages, message)
}

// GetMessages returns all messages.
func (s *AppState) GetMessages() []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	messages := make([]Message, len(s.Messages))
	copy(messages, s.Messages)
	return messages
}

// SetMode sets the application mode.
func (s *AppState) SetMode(mode string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Mode = mode
}

// GetMode returns the current mode.
func (s *AppState) GetMode() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.Mode
}

// SetPermissions sets the permission mode.
func (s *AppState) SetPermissions(perms string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Permissions = perms
}

// GetPermissions returns the permission mode.
func (s *AppState) GetPermissions() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.Permissions
}

// SetMetadata sets a metadata value.
func (s *AppState) SetMetadata(key string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metadata[key] = value
}

// GetMetadata retrieves a metadata value.
func (s *AppState) GetMetadata(key string) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	val, ok := s.metadata[key]
	return val, ok
}
