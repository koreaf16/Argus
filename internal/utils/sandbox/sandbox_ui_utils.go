// Package sandbox — Sandbox UI utilities for status display.
//
// 파일 역할: Provides UI helpers for displaying sandbox status and information.
//
//	For Phase 1, these are minimal stubs.
//
// 포함 모듈:
//   - FormatSandboxStatus: Formats sandbox status for display
//   - SandboxStatusMessage: Generates user-facing sandbox status messages
//
// 호출/사용 방식:
//   - Internal UI rendering (Phase 2)
//   - Logging and debugging output
//
// 연결:
//   - internal/utils/sandbox: sandbox_adapter.go
//   - 원본: src/utils/sandbox/ UI helpers (Phase 2)
package sandbox

import (
	"fmt"
	"time"
)

// SandboxStatus represents the current state of a sandbox.
type SandboxStatus struct {
	ID         string
	Name       string
	Status     string // "running", "idle", "failed"
	CreatedAt  time.Time
	LastUsed   time.Time
	Memory     int64 // bytes
	CPUPercent float64
	Error      string
}

// FormatSandboxStatus formats a sandbox status for display.
func FormatSandboxStatus(status *SandboxStatus) string {
	if status == nil {
		return "(no sandbox)"
	}

	return fmt.Sprintf(
		"Sandbox [%s] — Status: %s, Memory: %d MB, CPU: %.1f%%",
		status.ID,
		status.Status,
		status.Memory/(1024*1024),
		status.CPUPercent,
	)
}

// SandboxStatusMessage generates a user-facing status message.
func SandboxStatusMessage(status string, details string) string {
	switch status {
	case "running":
		return fmt.Sprintf("Sandbox running: %s", details)
	case "idle":
		return fmt.Sprintf("Sandbox idle: %s", details)
	case "failed":
		return fmt.Sprintf("Sandbox failed: %s", details)
	default:
		return fmt.Sprintf("Sandbox unknown state: %s", details)
	}
}

// FormatExecutionDetails formats execution details for display.
func FormatExecutionDetails(sandboxID, command string, exitCode int, duration time.Duration) string {
	return fmt.Sprintf(
		"Executed in %s [%s]: %s (exit code: %d)",
		sandboxID,
		duration.String(),
		command,
		exitCode,
	)
}

// SandboxWarning generates a warning message if sandboxing is not available.
func SandboxWarning() string {
	return "Sandboxing not available on this platform — proceeding without isolation"
}

// SandboxNotAvailableMessage returns the message when sandboxing is not available.
func SandboxNotAvailableMessage() string {
	return "Sandbox execution is not available in Phase 1. Command will execute normally."
}
