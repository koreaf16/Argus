package shelljobs

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type EventKind string

const (
	EventStart  EventKind = "start"
	EventOutput EventKind = "output"
	EventDone   EventKind = "done"
)

type JobStatus string

const (
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
)

type Event struct {
	Kind        EventKind `json:"kind"`
	JobID       string    `json:"job_id"`
	ToolName    string    `json:"tool_name"`
	TargetAlias string    `json:"target_alias,omitempty"`
	Command     string    `json:"command,omitempty"`
	Output      string    `json:"output,omitempty"`
	ExitCode    int       `json:"exit_code,omitempty"`
	Error       string    `json:"error,omitempty"`
}

type Snapshot struct {
	ID          string     `json:"id"`
	ToolName    string     `json:"tool_name"`
	TargetAlias string     `json:"target_alias,omitempty"`
	Command     string     `json:"command,omitempty"`
	Status      JobStatus  `json:"status"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	ExitCode    int        `json:"exit_code,omitempty"`
	Error       string     `json:"error,omitempty"`
}

type jobState struct {
	snapshot Snapshot
	output   string
	writeFn  func(string) error
	killFn   func()
}

type Manager struct {
	mu    sync.RWMutex
	jobs  map[string]*jobState
	subs  map[chan Event]struct{}
	next  atomic.Uint64
	limit int
}

func NewManager() *Manager {
	m := &Manager{
		jobs:  make(map[string]*jobState),
		subs:  make(map[chan Event]struct{}),
		limit: 1024 * 1024,
	}
	m.next.Store(1)
	return m
}

func (m *Manager) StartJob(
	toolName string,
	targetAlias string,
	command string,
	writeFn func(string) error,
	killFn func(),
	initialOutput string,
) Snapshot {
	id := fmt.Sprintf("job-%06d", m.next.Add(1))
	now := time.Now().UTC()
	state := &jobState{
		snapshot: Snapshot{
			ID:          id,
			ToolName:    strings.TrimSpace(toolName),
			TargetAlias: strings.TrimSpace(targetAlias),
			Command:     command,
			Status:      JobStatusRunning,
			StartedAt:   now,
		},
		writeFn: writeFn,
		killFn:  killFn,
	}

	if strings.TrimSpace(initialOutput) != "" {
		state.output = clampTail(initialOutput, m.limit)
	}

	m.mu.Lock()
	m.jobs[id] = state
	m.mu.Unlock()

	m.emit(Event{
		Kind:        EventStart,
		JobID:       id,
		ToolName:    state.snapshot.ToolName,
		TargetAlias: state.snapshot.TargetAlias,
		Command:     state.snapshot.Command,
	})
	if state.output != "" {
		m.emit(Event{
			Kind:        EventOutput,
			JobID:       id,
			ToolName:    state.snapshot.ToolName,
			TargetAlias: state.snapshot.TargetAlias,
			Output:      state.output,
		})
	}
	return state.snapshot
}

func (m *Manager) AppendOutput(jobID, chunk string) {
	if strings.TrimSpace(chunk) == "" {
		return
	}

	m.mu.Lock()
	state := m.jobs[jobID]
	if state != nil {
		state.output = clampTail(state.output+chunk, m.limit)
	}
	m.mu.Unlock()

	if state == nil {
		return
	}
	m.emit(Event{
		Kind:        EventOutput,
		JobID:       jobID,
		ToolName:    state.snapshot.ToolName,
		TargetAlias: state.snapshot.TargetAlias,
		Output:      chunk,
	})
}

func (m *Manager) Complete(jobID string, exitCode int, errText string) {
	m.mu.Lock()
	state := m.jobs[jobID]
	if state == nil {
		m.mu.Unlock()
		return
	}
	now := time.Now().UTC()
	state.snapshot.CompletedAt = &now
	state.snapshot.ExitCode = exitCode
	state.snapshot.Error = strings.TrimSpace(errText)
	if exitCode == 0 && state.snapshot.Error == "" {
		state.snapshot.Status = JobStatusCompleted
	} else {
		state.snapshot.Status = JobStatusFailed
	}
	snap := state.snapshot
	m.mu.Unlock()

	m.emit(Event{
		Kind:        EventDone,
		JobID:       jobID,
		ToolName:    snap.ToolName,
		TargetAlias: snap.TargetAlias,
		ExitCode:    snap.ExitCode,
		Error:       snap.Error,
	})
}

func (m *Manager) Stop(jobID string) error {
	m.mu.RLock()
	state := m.jobs[jobID]
	m.mu.RUnlock()
	if state == nil {
		return fmt.Errorf("job not found: %s", jobID)
	}
	if state.killFn == nil {
		return fmt.Errorf("job %s is not stoppable", jobID)
	}
	state.killFn()
	return nil
}

func (m *Manager) SendInput(jobID, input string) error {
	m.mu.RLock()
	state := m.jobs[jobID]
	m.mu.RUnlock()
	if state == nil {
		return fmt.Errorf("job not found: %s", jobID)
	}
	if state.writeFn == nil {
		return fmt.Errorf("job %s does not accept input", jobID)
	}
	return state.writeFn(input)
}

func (m *Manager) Snapshot(jobID string) (Snapshot, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state := m.jobs[jobID]
	if state == nil {
		return Snapshot{}, false
	}
	return state.snapshot, true
}

func (m *Manager) Tail(jobID string, maxChars int) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state := m.jobs[jobID]
	if state == nil {
		return "", fmt.Errorf("job not found: %s", jobID)
	}
	return clampTail(state.output, maxChars), nil
}

func (m *Manager) List() []Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Snapshot, 0, len(m.jobs))
	for _, state := range m.jobs {
		out = append(out, state.snapshot)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out
}

func (m *Manager) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 256)
	m.mu.Lock()
	m.subs[ch] = struct{}{}
	m.mu.Unlock()
	cancel := func() {
		m.mu.Lock()
		if _, ok := m.subs[ch]; ok {
			delete(m.subs, ch)
			close(ch)
		}
		m.mu.Unlock()
	}
	return ch, cancel
}

func (m *Manager) emit(evt Event) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for ch := range m.subs {
		select {
		case ch <- evt:
		default:
		}
	}
}

func clampTail(s string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	return string(runes[len(runes)-maxChars:])
}
