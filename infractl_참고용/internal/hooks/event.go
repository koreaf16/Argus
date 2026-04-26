// Package hooks
// File: event.go
// Description: Hook event input/output schema.
// Responsibility: Shared HookInput/HookOutput + Decision definitions.
package hooks

// HookEvent is the top-level hook event name in hooks.yaml.
type HookEvent string

const (
	HookEventPreToolUse   HookEvent = "PreToolUse"
	HookEventPostToolUse  HookEvent = "PostToolUse"
	HookEventSessionStart HookEvent = "SessionStart"
	HookEventSessionEnd   HookEvent = "SessionEnd"
)

// Decision is the PreToolUse hook verdict.
type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
	DecisionAsk   Decision = "ask"
)

// HookInput is passed to hook backends.
type HookInput struct {
	Event    HookEvent              `json:"event"`
	Tool     string                 `json:"tool"`
	Input    map[string]interface{} `json:"input,omitempty"`
	Output   map[string]interface{} `json:"output,omitempty"`
	Session  HookSession            `json:"session"`
	Metadata HookMetadata           `json:"metadata,omitempty"`
}

// HookSession describes the active session.
type HookSession struct {
	ID   string `json:"id"`
	User string `json:"user"`
	CWD  string `json:"cwd"`
}

// HookMetadata carries cheap analysis for hook matchers.
type HookMetadata struct {
	DiskModifying bool   `json:"disk_modifying,omitempty"`
	NetworkAccess bool   `json:"network_access,omitempty"`
	ReadOnly      bool   `json:"read_only,omitempty"`
	DangerScore   string `json:"danger_score,omitempty"`
}

// HookOutput is returned by a hook backend.
type HookOutput struct {
	Decision      Decision               `json:"decision,omitempty"`
	Reason        string                 `json:"reason,omitempty"`
	SystemMessage string                 `json:"systemMessage,omitempty"`
	NewInput      map[string]interface{} `json:"newInput,omitempty"`
	// RuntimeError is only set by the local runner when a backend call failed
	// and fail-closed synthesized a deny.
	RuntimeError bool `json:"-"`
}

// IsDeny reports whether the hook result should deny tool execution.
func (o HookOutput) IsDeny() bool {
	return o.Decision == DecisionDeny || o.Decision == DecisionAsk
}

// IsAllow reports whether the hook result allows tool execution.
func (o HookOutput) IsAllow() bool {
	return o.Decision == "" || o.Decision == DecisionAllow
}
