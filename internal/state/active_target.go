package state

import "strings"

const metaActiveTarget = "active_target"

// ActiveTarget is the single source of truth for the workspace and account
// context that the next bash tool invocation will run against. The TUI status
// bar and the LLM system prompt both read from this struct, so it must reflect
// the lane's actual cwd / user after every command.
type ActiveTarget struct {
	Alias        string
	LaneID       string
	AccountStack []string
	CWD          string
}

// EffectiveUser returns the top of the account stack, or "" if it is empty.
func (t ActiveTarget) EffectiveUser() string {
	if len(t.AccountStack) == 0 {
		return ""
	}
	return t.AccountStack[len(t.AccountStack)-1]
}

// Display returns "alias" or "alias:user" for status-bar / prompt rendering.
func (t ActiveTarget) Display() string {
	if t.Alias == "" {
		return ""
	}
	user := t.EffectiveUser()
	if user == "" {
		return t.Alias
	}
	return t.Alias + ":" + user
}

// SetActiveTarget stores the active target snapshot and fires the state-change
// hook when the user-visible representation changes. The struct is copied to
// prevent external mutation.
func (s *AppState) SetActiveTarget(t ActiveTarget) {
	cp := ActiveTarget{
		Alias:   strings.TrimSpace(t.Alias),
		LaneID:  strings.TrimSpace(t.LaneID),
		CWD:     t.CWD,
	}
	if len(t.AccountStack) > 0 {
		cp.AccountStack = make([]string, len(t.AccountStack))
		copy(cp.AccountStack, t.AccountStack)
	}

	s.mu.Lock()
	if s.Metadata == nil {
		s.Metadata = make(map[string]any)
	}
	before, _ := s.Metadata[metaActiveTarget].(ActiveTarget)
	s.Metadata[metaActiveTarget] = cp
	hook := s.onStateChange
	s.mu.Unlock()

	if hook != nil && before.Display() != cp.Display() {
		hook("active_target", before.Display(), cp.Display())
	}
}

// ActiveTarget returns a defensive copy of the current active target.
func (s *AppState) ActiveTarget() ActiveTarget {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return ActiveTarget{}
	}
	t, _ := s.Metadata[metaActiveTarget].(ActiveTarget)
	if len(t.AccountStack) > 0 {
		stack := make([]string, len(t.AccountStack))
		copy(stack, t.AccountStack)
		t.AccountStack = stack
	}
	return t
}
