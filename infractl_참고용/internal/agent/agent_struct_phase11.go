// Package agent
// File: agent_struct_phase11.go
// Description: Phase 11 TaskContext·ElevationTracker·Vault 필드의 setter 및 이벤트 헬퍼
// Responsibility: taskMgr/elevationTrk/credVault 주입, TaskEventSink 이벤트 헬퍼

package agent

import (
	"github.com/yourorg/infractl/internal/agent/taskctx"
	"github.com/yourorg/infractl/internal/privilege"
)

// SetTaskManager injects the TaskContext manager into the agent.
func (a *Agent) SetTaskManager(m *taskctx.Manager) { a.taskMgr = m }

// TaskManager returns the current TaskContext manager (may be nil).
func (a *Agent) TaskManager() *taskctx.Manager { return a.taskMgr }

// SetElevationTracker injects the privilege elevation tracker.
func (a *Agent) SetElevationTracker(t *privilege.Tracker) { a.elevationTrk = t }

// SetCredVault injects the session-scoped credential vault.
func (a *Agent) SetCredVault(v *privilege.Vault) { a.credVault = v }

// notifyTaskProposed fires OnTaskProposed if handler implements TaskEventSink.
func (a *Agent) notifyTaskProposed(title, kind, server, account string) {
	if s, ok := a.handler.(TaskEventSink); ok {
		s.OnTaskProposed(title, kind, server, account)
	}
}

// notifyTaskDeclared fires OnTaskDeclared if handler implements TaskEventSink.
func (a *Agent) notifyTaskDeclared(taskID, title, kind string) {
	if s, ok := a.handler.(TaskEventSink); ok {
		s.OnTaskDeclared(taskID, title, kind)
	}
}

// notifyTaskEnded fires OnTaskEnded if handler implements TaskEventSink.
func (a *Agent) notifyTaskEnded(taskID, status, summary string) {
	if s, ok := a.handler.(TaskEventSink); ok {
		s.OnTaskEnded(taskID, status, summary)
	}
}

// notifyElevationChanged fires OnElevationChanged if handler implements TaskEventSink.
func (a *Agent) notifyElevationChanged(host, sessionID, currentUser string) {
	if s, ok := a.handler.(TaskEventSink); ok {
		s.OnElevationChanged(host, sessionID, currentUser)
	}
}
