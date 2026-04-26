// Package installpath
// File: types.go
// Description: Types for install/migrate happy-path orchestration.
// Responsibility: Define pipeline state, events, and next-action contract.

package installpath

import "github.com/yourorg/infractl/internal/llm"

// InstallEvent represents a state transition source inside the pipeline.
type InstallEvent string

const (
	EventToolEnd     InstallEvent = "tool_end"
	EventJobComplete InstallEvent = "job_complete"
)

// NextAction is a runtime-generated follow-up call decision.
type NextAction struct {
	AutoInvoke bool
	Tool       string
	Args       map[string]any
	Reason     string
}

// PendingVerify tracks a submitted background job waiting for auto verification.
type PendingVerify struct {
	JobID   int
	TaskKey string
}

// PipelineState keeps mutable install pipeline progress.
type PipelineState struct {
	LastJobID     int
	LastShellMeta string
	PendingVerify PendingVerify
}

// QueuedAction is an enqueued auto tool call.
type QueuedAction struct {
	ToolCall llm.ToolCall
	Reason   string
}
