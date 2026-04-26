// Package todo
// File: tracker_test.go
// Description: Tracker 불변식 검증 테스트
// Responsibility: 6+ invariant 위반 케이스 커버

package todo

import (
	"testing"
)

func newTrackerWithItems(t *testing.T, items ...TodoItem) *Tracker {
	t.Helper()
	tr := NewTracker(NewStore())
	for _, it := range items {
		if err := tr.store.Add(it); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	return tr
}

func TestTracker_AddDupID(t *testing.T) {
	tr := newTrackerWithItems(t)
	item := TodoItem{ID: "1", Content: "a", Status: StatusPending}
	if err := tr.Add(item); err != nil {
		t.Fatal(err)
	}
	if err := tr.Add(item); err == nil {
		t.Fatal("expected error on duplicate id")
	}
}

func TestTracker_AddUnknownDep(t *testing.T) {
	tr := newTrackerWithItems(t)
	item := TodoItem{ID: "2", Content: "b", Status: StatusPending, Deps: []string{"nonexistent"}}
	if err := tr.Add(item); err == nil {
		t.Fatal("expected error for unknown dep")
	}
}

func TestTracker_SetInProgress_AlreadyOne(t *testing.T) {
	tr := newTrackerWithItems(t,
		TodoItem{ID: "a", Content: "a", Status: StatusInProgress},
		TodoItem{ID: "b", Content: "b", Status: StatusPending},
	)
	if err := tr.SetInProgress("b"); err == nil {
		t.Fatal("expected error: another item already in_progress")
	}
}

func TestTracker_SetInProgress_DepNotCompleted(t *testing.T) {
	tr := newTrackerWithItems(t,
		TodoItem{ID: "dep", Content: "dep", Status: StatusPending},
		TodoItem{ID: "child", Content: "child", Status: StatusPending, Deps: []string{"dep"}},
	)
	if err := tr.SetInProgress("child"); err == nil {
		t.Fatal("expected error: dep not completed")
	}
}

func TestTracker_SetInProgress_HappyPath(t *testing.T) {
	tr := newTrackerWithItems(t,
		TodoItem{ID: "dep", Content: "dep", Status: StatusCompleted},
		TodoItem{ID: "child", Content: "child", Status: StatusPending, Deps: []string{"dep"}},
	)
	if err := tr.SetInProgress("child"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	it, _ := tr.Store().Get("child")
	if it.Status != StatusInProgress {
		t.Errorf("expected in_progress, got %s", it.Status)
	}
}

func TestTracker_SetCompleted_AlreadyCompleted(t *testing.T) {
	tr := newTrackerWithItems(t,
		TodoItem{ID: "x", Content: "x", Status: StatusCompleted},
	)
	if err := tr.SetCompleted("x"); err == nil {
		t.Fatal("expected error: already completed (immutable)")
	}
}

func TestTracker_SetCompleted_NotFound(t *testing.T) {
	tr := newTrackerWithItems(t)
	if err := tr.SetCompleted("ghost"); err == nil {
		t.Fatal("expected error: item not found")
	}
}

func TestTracker_SetInProgress_NotFound(t *testing.T) {
	tr := newTrackerWithItems(t)
	if err := tr.SetInProgress("ghost"); err == nil {
		t.Fatal("expected error: item not found")
	}
}
