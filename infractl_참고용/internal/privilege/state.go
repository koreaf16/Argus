// Package privilege
// File: state.go
// Description: 세션별 권한 상승 상태 추적
// Responsibility: ElevationState 구조체와 IsRoot/CoversAccount/Clone 판정 로직

package privilege

import "time"

// ElevationHop records a single step in the privilege escalation chain.
type ElevationHop struct {
	From   string
	To     string
	Method Method
	At     time.Time
}

// ElevationState represents the current privilege escalation status for one session.
type ElevationState struct {
	SessionID    string
	TargetHost   string
	OriginalUser string
	CurrentUser  string
	ElevatedFrom string
	Method       Method
	ElevatedAt   time.Time
	ActiveSince  time.Time
	Chain        []ElevationHop
}

// IsRoot reports whether the current effective user is root.
func (s *ElevationState) IsRoot() bool {
	if s == nil {
		return false
	}
	return s.CurrentUser == "root"
}

// CoversAccount reports whether the current privilege level covers the target account.
// Returns true when CurrentUser equals targetAccount or CurrentUser is root.
func (s *ElevationState) CoversAccount(targetAccount string) bool {
	if s == nil {
		return false
	}
	return s.CurrentUser == "root" || s.CurrentUser == targetAccount
}

// Clone returns a deep copy of the ElevationState including a new Chain slice.
func (s *ElevationState) Clone() ElevationState {
	if s == nil {
		return ElevationState{}
	}
	clone := *s
	if len(s.Chain) > 0 {
		clone.Chain = make([]ElevationHop, len(s.Chain))
		copy(clone.Chain, s.Chain)
	} else {
		clone.Chain = nil
	}
	return clone
}
