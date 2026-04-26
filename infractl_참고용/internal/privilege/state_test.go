// Package privilege
// File: state_test.go
// Description: ElevationState 및 Tracker 순수 로직 단위 테스트
// Responsibility: IsRoot, CoversAccount, OnElevation 체인, OnProbe, Snapshot 검증

package privilege

import (
	"testing"
	"time"
)

// --- ElevationState ---

func TestIsRoot_True(t *testing.T) {
	s := &ElevationState{CurrentUser: "root"}
	if !s.IsRoot() {
		t.Fatal("expected IsRoot to return true for CurrentUser=root")
	}
}

func TestIsRoot_False(t *testing.T) {
	s := &ElevationState{CurrentUser: "oracle"}
	if s.IsRoot() {
		t.Fatal("expected IsRoot to return false for CurrentUser=oracle")
	}
}

func TestIsRoot_Nil(t *testing.T) {
	var s *ElevationState
	if s.IsRoot() {
		t.Fatal("expected IsRoot to return false for nil receiver")
	}
}

func TestCoversAccount_RootCoversAll(t *testing.T) {
	s := &ElevationState{CurrentUser: "root"}
	for _, acc := range []string{"oracle", "mysql", "root", "postgres"} {
		if !s.CoversAccount(acc) {
			t.Fatalf("expected root to cover account %q", acc)
		}
	}
}

func TestCoversAccount_SameUser(t *testing.T) {
	s := &ElevationState{CurrentUser: "oracle"}
	if !s.CoversAccount("oracle") {
		t.Fatal("expected oracle to cover oracle")
	}
}

func TestCoversAccount_OtherUser(t *testing.T) {
	s := &ElevationState{CurrentUser: "oracle"}
	for _, acc := range []string{"root", "mysql"} {
		if s.CoversAccount(acc) {
			t.Fatalf("expected oracle NOT to cover account %q", acc)
		}
	}
}

func TestCoversAccount_Nil(t *testing.T) {
	var s *ElevationState
	if s.CoversAccount("root") {
		t.Fatal("expected nil to cover nothing")
	}
}

// --- Tracker.OnElevation chain ---

func TestTracker_OnElevation_Chain(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tr := &Tracker{
		states: make(map[string]*ElevationState),
		clock:  func() time.Time { return now },
	}

	const (
		host      = "dbhost"
		sessionID = "sess1"
	)

	tr.OnSessionCreated(host, sessionID, "admin")

	// admin → root via sudo
	tr.OnElevation(host, sessionID, MethodSudo, "admin", "root")
	// root → oracle via su
	tr.OnElevation(host, sessionID, MethodSU, "root", "oracle")

	snap, ok := tr.Snapshot(host, sessionID)
	if !ok {
		t.Fatal("expected snapshot to be present")
	}

	if len(snap.Chain) != 2 {
		t.Fatalf("expected chain length 2, got %d", len(snap.Chain))
	}
	if snap.CurrentUser != "oracle" {
		t.Fatalf("expected CurrentUser=oracle, got %q", snap.CurrentUser)
	}
	if snap.ElevatedFrom != "root" {
		t.Fatalf("expected ElevatedFrom=root, got %q", snap.ElevatedFrom)
	}
	if snap.Chain[0].From != "admin" || snap.Chain[0].To != "root" {
		t.Fatalf("unexpected chain[0]: %+v", snap.Chain[0])
	}
	if snap.Chain[1].From != "root" || snap.Chain[1].To != "oracle" {
		t.Fatalf("unexpected chain[1]: %+v", snap.Chain[1])
	}
}

// --- Tracker.OnProbe ---

func TestTracker_OnProbe_SyncsCurrentUser(t *testing.T) {
	now := time.Now()
	tr := &Tracker{
		states: make(map[string]*ElevationState),
		clock:  func() time.Time { return now },
	}
	tr.OnSessionCreated("h", "s", "admin")
	tr.OnElevation("h", "s", MethodSudo, "admin", "root")

	tr.OnProbe("h", "s", "oracle", "/home/oracle")

	snap, _ := tr.Snapshot("h", "s")
	if snap.CurrentUser != "oracle" {
		t.Fatalf("expected CurrentUser=oracle after probe, got %q", snap.CurrentUser)
	}
	// Chain must remain unchanged (only 1 hop from the elevation above)
	if len(snap.Chain) != 1 {
		t.Fatalf("expected chain length 1 after probe, got %d", len(snap.Chain))
	}
}

// --- Tracker.Snapshot deep copy ---

func TestTracker_Snapshot_DeepCopy(t *testing.T) {
	now := time.Now()
	tr := &Tracker{
		states: make(map[string]*ElevationState),
		clock:  func() time.Time { return now },
	}
	tr.OnSessionCreated("h", "s", "admin")
	tr.OnElevation("h", "s", MethodSudo, "admin", "root")

	snap, _ := tr.Snapshot("h", "s")

	// mutate the returned snapshot's Chain
	snap.Chain[0].To = "MUTATED"
	snap.CurrentUser = "MUTATED"

	// sanity: local mutation took effect on our copy
	if snap.CurrentUser != "MUTATED" || snap.Chain[0].To != "MUTATED" {
		t.Fatal("local snapshot mutation failed")
	}

	// live state must be unchanged
	live, _ := tr.Get("h", "s")
	if live.Chain[0].To == "MUTATED" {
		t.Fatal("snapshot mutation affected live Chain")
	}
	if live.CurrentUser == "MUTATED" {
		t.Fatal("snapshot mutation affected live CurrentUser")
	}
}
