// Package agent
// File: pending_action.go
// Description: LLM이 제안한 명령/계획을 추적하여 사용자 확인 응답에 즉시 매칭·실행
// Responsibility: 제안 활성화(Activate), 소비(Consume), 상태 유효성 관리

package agent

import (
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const (
	pendingActionStaleTurns    = 3
	pendingActionStaleDuration = 5 * time.Minute
)

// PendingAction은 LLM이 제안한 명령 하나와 후속 계획을 담는다.
type PendingAction struct {
	ProposedCommand string
	Plan            []string
	Reason          string
	ServerName      string
	CreatedAt       time.Time
	TurnIndex       int
	Source          string // "tool"
}

// ProposeActionMeta는 propose_action 도구가 MetadataJSON에 직렬화하는 페이로드다.
// agent가 이를 읽어 PendingAction을 활성화한다.
type ProposeActionMeta struct {
	Command string   `json:"command"`
	Reason  string   `json:"reason"`
	Plan    []string `json:"plan,omitempty"`
}

// PendingActionTracker는 직전 LLM 제안을 추적하는 인터페이스다.
type PendingActionTracker interface {
	Activate(p *PendingAction, turnIndex int)
	Current() *PendingAction
	Consume() *PendingAction
	Clear(reason string)
	IsStale(currentTurnIndex int) bool
	ParseFromMeta(metaJSON string) (*PendingAction, bool)
}

type pendingActionTracker struct {
	mu      sync.RWMutex
	current *PendingAction
}

// newPendingActionTracker는 빈 PendingActionTracker 구현체를 반환한다.
func newPendingActionTracker() PendingActionTracker {
	return &pendingActionTracker{}
}

func (t *pendingActionTracker) Activate(p *PendingAction, turnIndex int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	p.TurnIndex = turnIndex
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	t.current = p
	slog.Info("pending action activated",
		"command_preview", truncatePendingPreview(p.ProposedCommand, 80),
		"plan_steps", len(p.Plan),
		"source", p.Source)
}

func (t *pendingActionTracker) Current() *PendingAction {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.current
}

func (t *pendingActionTracker) Consume() *PendingAction {
	t.mu.Lock()
	defer t.mu.Unlock()
	p := t.current
	t.current = nil
	if p != nil {
		slog.Info("pending action consumed", "turn_index", p.TurnIndex)
	}
	return p
}

func (t *pendingActionTracker) Clear(reason string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.current != nil {
		slog.Debug("pending action cleared", "reason", reason)
	}
	t.current = nil
}

func (t *pendingActionTracker) IsStale(currentTurnIndex int) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.current == nil {
		return true
	}
	if currentTurnIndex-t.current.TurnIndex >= pendingActionStaleTurns {
		return true
	}
	if time.Since(t.current.CreatedAt) >= pendingActionStaleDuration {
		return true
	}
	return false
}

// ParseFromMeta는 propose_action 도구의 MetadataJSON에서 PendingAction을 복원한다.
func (t *pendingActionTracker) ParseFromMeta(metaJSON string) (*PendingAction, bool) {
	if strings.TrimSpace(metaJSON) == "" {
		return nil, false
	}
	var meta ProposeActionMeta
	if err := json.Unmarshal([]byte(metaJSON), &meta); err != nil {
		return nil, false
	}
	if strings.TrimSpace(meta.Command) == "" {
		return nil, false
	}
	return &PendingAction{
		ProposedCommand: strings.TrimSpace(meta.Command),
		Reason:          strings.TrimSpace(meta.Reason),
		Plan:            meta.Plan,
		Source:          "tool",
		CreatedAt:       time.Now(),
	}, true
}

func truncatePendingPreview(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
