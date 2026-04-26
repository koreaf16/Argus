// Package todo
// File: store.go
// Description: TodoItem 타입 정의 및 인메모리 세션 스토어
// Responsibility: 세션 스코프 todo 목록 CRUD — 동시성 안전 맵

package todo

import (
	"fmt"
	"sync"
)

// Status 는 todo 항목의 상태이다.
type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
)

// TodoItem 은 단일 작업 항목이다.
type TodoItem struct {
	ID      string
	Content string
	Status  Status
	Deps    []string // 선행 완료 필요 item ID 목록
}

// Store 는 세션 스코프 인메모리 todo 스토어이다.
type Store struct {
	mu    sync.RWMutex
	items map[string]*TodoItem
	order []string
}

// NewStore 는 빈 Store 를 반환한다.
func NewStore() *Store {
	return &Store{items: make(map[string]*TodoItem)}
}

// Add 는 새 TodoItem 을 추가한다. ID 중복 시 에러를 반환한다.
func (s *Store) Add(item TodoItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.items[item.ID]; exists {
		return fmt.Errorf("todo: duplicate id %q", item.ID)
	}
	clone := item
	s.items[item.ID] = &clone
	s.order = append(s.order, item.ID)
	return nil
}

// Update 는 기존 항목의 Status 를 갱신한다. 항목이 없으면 에러를 반환한다.
func (s *Store) Update(id string, status Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return fmt.Errorf("todo: item %q not found", id)
	}
	item.Status = status
	return nil
}

// List 는 삽입 순서대로 모든 항목의 복사본을 반환한다.
func (s *Store) List() []TodoItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]TodoItem, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, *s.items[id])
	}
	return out
}

// Get 은 ID 로 항목을 반환한다.
func (s *Store) Get(id string) (TodoItem, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	if !ok {
		return TodoItem{}, false
	}
	return *item, true
}
