// Package web
// File: handler.go
// Description: Web UI용 EventHandler 및 TaskEventSink 스텁 구현
// Responsibility: WebEventHandler — agent 이벤트를 Web UI로 전달하는 스텁 (Phase 7에서 구현 예정)

package web

import (
	"log/slog"
	"time"
)

// WebEventHandler is a stub implementation of agent.EventHandler and agent.TaskEventSink.
// All methods are no-ops until Phase 7 (Web UI) implements them.
type WebEventHandler struct{}

// NewWebEventHandler creates a no-op web event handler.
func NewWebEventHandler() *WebEventHandler { return &WebEventHandler{} }

func (h *WebEventHandler) OnThinking(tier string, model string) {
	slog.Debug("web: on thinking", "tier", tier, "model", model)
}

func (h *WebEventHandler) OnThinkingToken(_ string) {}

func (h *WebEventHandler) OnToken(_ string) {}

func (h *WebEventHandler) OnToolStart(toolID string, toolName string, target string, args map[string]any) {
	slog.Debug("web: tool start", "id", toolID, "tool", toolName, "target", target)
}

func (h *WebEventHandler) OnToolOutput(_ string, _ string) {}

func (h *WebEventHandler) OnToolEnd(toolID string, toolName string, result string, duration time.Duration, success bool, _ string) {
	slog.Debug("web: tool end", "id", toolID, "tool", toolName, "success", success, "dur", duration)
}

func (h *WebEventHandler) OnToolCancelled(toolID string, toolName string, reason string, duration time.Duration) {
	slog.Debug("web: tool cancelled", "id", toolID, "tool", toolName, "reason", reason, "dur", duration)
}

func (h *WebEventHandler) OnResponse(content string) {
	slog.Debug("web: response", "len", len(content))
}

func (h *WebEventHandler) OnError(err error) {
	slog.Error("web: agent error", "err", err)
}

func (h *WebEventHandler) OnUsageUpdate(inputTokens, outputTokens int, costUSD float64, durationMs int64) {
	slog.Debug("web: usage update", "input", inputTokens, "output", outputTokens, "cost", costUSD)
}

func (h *WebEventHandler) OnJobComplete(jobID int, description string, success bool) {
	slog.Debug("web: job complete", "job", jobID, "desc", description, "success", success)
}

func (h *WebEventHandler) OnRAGContext(_ int) {}

// TaskEventSink stubs — will be wired to Web UI in Phase 7.

func (h *WebEventHandler) OnTaskProposed(title, kind, server, account string) {
	slog.Debug("web: task proposed", "title", title, "kind", kind)
}

func (h *WebEventHandler) OnTaskDeclared(taskID, title, kind string) {
	slog.Debug("web: task declared", "id", taskID, "title", title)
}

func (h *WebEventHandler) OnTaskStepAdvanced(taskID string, stepIndex, total int, description string) {
	slog.Debug("web: task step", "id", taskID, "step", stepIndex, "total", total)
}

func (h *WebEventHandler) OnTaskEnded(taskID, status, summary string) {
	slog.Debug("web: task ended", "id", taskID, "status", status)
}

func (h *WebEventHandler) OnElevationChanged(host, sessionID, currentUser string) {
	slog.Debug("web: elevation changed", "host", host, "session", sessionID, "user", currentUser)
}

func (h *WebEventHandler) OnGuardViolation(toolName, reason string, blocked bool) {
	slog.Debug("web: guard violation", "tool", toolName, "blocked", blocked)
}
