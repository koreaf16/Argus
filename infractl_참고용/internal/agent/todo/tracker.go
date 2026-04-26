// Package todo
// File: tracker.go
// Description: TodoItem 상태 전이 불변식 검증기
// Responsibility: in_progress 1개 제한, deps 미충족 거부, completed 불변 강제

package todo

import "fmt"

// Tracker 는 Store 위에서 불변식을 강제한다.
type Tracker struct {
	store *Store
}

// NewTracker 는 Store 를 래핑하는 Tracker 를 반환한다.
func NewTracker(store *Store) *Tracker {
	return &Tracker{store: store}
}

// Add 는 불변식 검사 후 store 에 추가한다.
// deps 에 명시된 ID 가 스토어에 없으면 에러를 반환한다.
func (t *Tracker) Add(item TodoItem) error {
	items := t.store.List()
	for _, dep := range item.Deps {
		found := false
		for _, existing := range items {
			if existing.ID == dep {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("todo: dep %q not found for item %q", dep, item.ID)
		}
	}
	return t.store.Add(item)
}

// SetInProgress 는 항목을 in_progress 로 전환한다.
// 이미 다른 항목이 in_progress 이면 에러를 반환한다.
// deps 가 모두 completed 가 아니면 에러를 반환한다.
func (t *Tracker) SetInProgress(id string) error {
	items := t.store.List()
	for _, it := range items {
		if it.ID != id && it.Status == StatusInProgress {
			return fmt.Errorf("todo: item %q is already in_progress", it.ID)
		}
	}
	target, ok := t.store.Get(id)
	if !ok {
		return fmt.Errorf("todo: item %q not found", id)
	}
	for _, dep := range target.Deps {
		depItem, depOK := t.store.Get(dep)
		if !depOK || depItem.Status != StatusCompleted {
			return fmt.Errorf("todo: dep %q not completed for item %q", dep, id)
		}
	}
	return t.store.Update(id, StatusInProgress)
}

// SetCompleted 는 항목을 completed 로 전환한다.
// 항목이 이미 completed 이면 에러를 반환한다 (불변).
func (t *Tracker) SetCompleted(id string) error {
	item, ok := t.store.Get(id)
	if !ok {
		return fmt.Errorf("todo: item %q not found", id)
	}
	if item.Status == StatusCompleted {
		return fmt.Errorf("todo: item %q is already completed", id)
	}
	return t.store.Update(id, StatusCompleted)
}

// Store 는 내부 스토어를 반환한다.
func (t *Tracker) Store() *Store { return t.store }
