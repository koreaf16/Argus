// Package privilege
// File: state_tracker.go
// Description: 세션별 ElevationState를 스레드 안전하게 관리하는 Tracker
// Responsibility: OnSessionCreated/OnElevation/OnProbe 이벤트 처리 및 상태 조회

package privilege

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Tracker manages ElevationState for multiple sessions in a thread-safe manner.
type Tracker struct {
	mu     sync.RWMutex
	states map[string]*ElevationState
	clock  func() time.Time
}

// NewTracker creates a new Tracker with the default wall clock.
func NewTracker() *Tracker {
	return &Tracker{
		states: make(map[string]*ElevationState),
		clock:  time.Now,
	}
}

// trackerKey returns the map key for a host/session pair.
func trackerKey(host, sessionID string) string {
	return fmt.Sprintf("%s|%s", host, sessionID)
}

// OnSessionCreated registers a new session with the initial login user.
// Method is set to MethodNone and both ElevatedAt and ActiveSince are set to now.
func (t *Tracker) OnSessionCreated(host, sessionID, loginUser string) {
	now := t.clock()
	key := trackerKey(host, sessionID)
	state := &ElevationState{
		SessionID:    sessionID,
		TargetHost:   host,
		OriginalUser: loginUser,
		CurrentUser:  loginUser,
		ElevatedFrom: "",
		Method:       MethodNone,
		ElevatedAt:   now,
		ActiveSince:  now,
		Chain:        nil,
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.states[key] = state
}

// OnElevation records a privilege escalation hop and updates the current user.
func (t *Tracker) OnElevation(host, sessionID string, method Method, from, to string) {
	key := trackerKey(host, sessionID)
	now := t.clock()
	t.mu.Lock()
	defer t.mu.Unlock()
	state, ok := t.states[key]
	if !ok {
		// create a minimal state if the session was not registered beforehand
		state = &ElevationState{
			SessionID:   sessionID,
			TargetHost:  host,
			ActiveSince: now,
		}
		t.states[key] = state
	}
	hop := ElevationHop{
		From:   from,
		To:     to,
		Method: method,
		At:     now,
	}
	state.Chain = append(state.Chain, hop)
	state.ElevatedFrom = state.CurrentUser
	state.CurrentUser = to
	state.Method = method
	state.ElevatedAt = now
}

// OnProbe synchronises the CurrentUser based on an observed shell state.
// It only updates CurrentUser when the observed value differs — Chain is left unchanged.
func (t *Tracker) OnProbe(host, sessionID, observedUser, _ string) {
	key := trackerKey(host, sessionID)
	t.mu.Lock()
	defer t.mu.Unlock()
	state, ok := t.states[key]
	if !ok {
		return
	}
	if state.CurrentUser != observedUser {
		state.CurrentUser = observedUser
	}
}

// Get returns the live pointer to the ElevationState for the given session.
// Callers must treat the returned value as read-only.
func (t *Tracker) Get(host, sessionID string) (*ElevationState, bool) {
	key := trackerKey(host, sessionID)
	t.mu.RLock()
	defer t.mu.RUnlock()
	s, ok := t.states[key]
	return s, ok
}

// Snapshot returns a deep copy of the ElevationState so the caller can safely
// inspect or modify it without affecting the live state.
func (t *Tracker) Snapshot(host, sessionID string) (ElevationState, bool) {
	key := trackerKey(host, sessionID)
	t.mu.RLock()
	defer t.mu.RUnlock()
	s, ok := t.states[key]
	if !ok {
		return ElevationState{}, false
	}
	return s.Clone(), true
}

// Remove deletes the session from the tracker, releasing its resources.
func (t *Tracker) Remove(host, sessionID string) {
	key := trackerKey(host, sessionID)
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.states, key)
}

// OnSessionClosed removes the elevation state when a session closes.
// Implements ssh.ElevationEventSink (duck-typed; no import needed).
func (t *Tracker) OnSessionClosed(host, sessionID string) {
	t.Remove(host, sessionID)
}

// OnElevationDone implements ssh.ElevationEventSink by mapping to OnElevation.
// methodStr is a free-form string from the SSH layer; "sudo" prefix selects MethodSudo.
func (t *Tracker) OnElevationDone(host, sessionID, methodStr, fromUser, toUser string) {
	m := MethodSU
	if strings.HasPrefix(strings.ToLower(methodStr), "sudo") {
		m = MethodSudo
	}
	t.OnElevation(host, sessionID, m, fromUser, toUser)
}
