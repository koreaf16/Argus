package state

import (
	"sync"
	"time"

	"github.com/koreaf16/argus/internal/types"
)

// AppState represents the entire application state.
type AppState struct {
	mu            sync.RWMutex
	Messages      []Message
	Mode          string // "normal", "plan"
	Permissions   string // "ask", "allow", "deny"
	Metadata      map[string]interface{}
	onStateChange func(field, before, after string)
}

// SetStateChangeHook registers a callback invoked after any state field changes.
func (s *AppState) SetStateChangeHook(fn func(field, before, after string)) {
	s.mu.Lock()
	s.onStateChange = fn
	s.mu.Unlock()
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
		Metadata:    make(map[string]interface{}),
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
	before := s.Mode
	s.Mode = mode
	hook := s.onStateChange
	s.mu.Unlock()
	if hook != nil && before != mode {
		hook("plan_mode", before, mode)
	}
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
	if s.Metadata == nil {
		s.Metadata = make(map[string]interface{})
	}
	s.Metadata[key] = value
}

// GetMetadata retrieves a metadata value.
func (s *AppState) GetMetadata(key string) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return nil, false
	}
	val, ok := s.Metadata[key]
	return val, ok
}

// MetadataSnapshot returns a detached copy of session metadata for persistence.
func (s *AppState) MetadataSnapshot() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return nil
	}
	out := make(map[string]interface{}, len(s.Metadata))
	for key, val := range s.Metadata {
		out[key] = cloneMetadataValue(val)
	}
	return out
}

func cloneMetadataValue(v interface{}) interface{} {
	switch x := v.(type) {
	case *WorkflowCard:
		return cloneWorkflowCard(x)
	case WorkflowCard:
		return cloneWorkflowCard(&x)
	case []string:
		return append([]string(nil), x...)
	case []types.PermissionRule:
		return append([]types.PermissionRule(nil), x...)
	case []types.TodoItem:
		return append([]types.TodoItem(nil), x...)
	case map[string][]types.TodoItem:
		cp := make(map[string][]types.TodoItem, len(x))
		for key, items := range x {
			cp[key] = append([]types.TodoItem(nil), items...)
		}
		return cp
	case map[string]interface{}:
		cp := make(map[string]interface{}, len(x))
		for key, val := range x {
			cp[key] = cloneMetadataValue(val)
		}
		return cp
	default:
		return x
	}
}
