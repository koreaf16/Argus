// Package taskctx
// File: manager.go
// Description: TaskContext 생명주기 관리
// Responsibility: Declare/Current/AttachElevation/Note/End/Snapshot/History, thread-safe

package taskctx

import (
	"fmt"
	"log/slog"
	"maps"
	"sync"
	"time"

	"github.com/yourorg/infractl/internal/privilege"
)

const maxHistory = 5

// Manager 는 TaskContext 생명주기를 thread-safe 하게 관리한다.
type Manager struct {
	mu         sync.RWMutex
	current    *TaskContext
	guardrails *Guardrails
	history    []*TaskContext // 최근 maxHistory 개 유지
	clock      func() time.Time
}

// NewManager 는 Manager 를 초기화한다.
func NewManager() *Manager {
	return &Manager{
		clock: time.Now,
	}
}

// DeclareRequest 는 새 TaskContext 선언에 필요한 파라미터이다.
type DeclareRequest struct {
	Title         string
	Kind          TaskKind
	TargetServer  string
	TargetAccount string
	WorkingDir    string
	Forbidden     []string
	Preconditions []string
	PlanSteps     []string
	KindFields    map[string]string
}

// Declare 는 새 TaskContext 를 시작한다.
// 기존 active가 있으면 먼저 End("aborted") 처리.
// Forbidden regex 컴파일 실패 시 에러 반환.
func (m *Manager) Declare(req DeclareRequest) (*TaskContext, error) {
	// Forbidden 패턴을 미리 컴파일 (fail-closed)
	gr, err := CompileForbidden(req.Forbidden)
	if err != nil {
		return nil, fmt.Errorf("declare task: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 기존 active 가 있으면 aborted 처리 후 history 이동
	if m.current != nil && m.current.Status == TaskActive {
		m.endLocked(TaskAborted)
	}

	tc := &TaskContext{
		ID:            NewID(),
		Title:         req.Title,
		Kind:          req.Kind,
		TargetServer:  req.TargetServer,
		TargetAccount: req.TargetAccount,
		WorkingDir:    req.WorkingDir,
		Status:        TaskActive,
		CreatedAt:     m.clock(),
	}

	if len(req.Forbidden) > 0 {
		tc.Forbidden = make([]string, len(req.Forbidden))
		copy(tc.Forbidden, req.Forbidden)
	}
	if len(req.Preconditions) > 0 {
		tc.Preconditions = make([]string, len(req.Preconditions))
		copy(tc.Preconditions, req.Preconditions)
	}
	if len(req.PlanSteps) > 0 {
		tc.PlanSteps = make([]string, len(req.PlanSteps))
		copy(tc.PlanSteps, req.PlanSteps)
	}
	if req.KindFields != nil {
		tc.KindFields = make(map[string]string, len(req.KindFields))
		maps.Copy(tc.KindFields, req.KindFields)
	}

	m.current = tc
	m.guardrails = gr

	slog.Info("task declared", "id", tc.ID, "title", tc.Title, "kind", tc.Kind,
		"server", tc.TargetServer, "account", tc.TargetAccount)

	return tc, nil
}

// Current 는 현재 활성 TaskContext 를 반환한다 (nil 가능).
func (m *Manager) Current() *TaskContext {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// Guardrails 는 현재 컴파일된 Guardrails 를 반환한다 (nil 가능).
func (m *Manager) Guardrails() *Guardrails {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.guardrails
}

// AttachElevation 은 현재 TaskContext 에 ElevationState 를 연결한다.
func (m *Manager) AttachElevation(es *privilege.ElevationState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return
	}
	m.current.Elevation = es
}

// Note 는 현재 TaskContext 에 노트를 추가한다.
func (m *Manager) Note(kind, msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return
	}
	m.current.AppendNote(kind, msg)
}

// End 는 현재 TaskContext 를 주어진 상태로 종료하고 history 에 이동한다.
func (m *Manager) End(status TaskStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return
	}
	m.endLocked(status)
}

// endLocked 는 lock 을 이미 잡은 상태에서 호출한다.
func (m *Manager) endLocked(status TaskStatus) {
	now := m.clock()
	m.current.Status = status
	m.current.EndedAt = &now

	slog.Info("task ended", "id", m.current.ID, "status", status)

	m.history = append(m.history, m.current)
	if len(m.history) > maxHistory {
		m.history = m.history[len(m.history)-maxHistory:]
	}

	m.current = nil
	m.guardrails = nil
}

// Snapshot 은 현재 TaskContext 의 불변 복사본을 반환한다.
func (m *Manager) Snapshot() TaskContextSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return snapshotFrom(m.current)
}

// History 는 최근 종료된 TaskContext 의 스냅샷 목록을 반환한다.
func (m *Manager) History() []TaskContextSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]TaskContextSnapshot, len(m.history))
	for i, tc := range m.history {
		result[i] = snapshotFrom(tc)
	}
	return result
}
