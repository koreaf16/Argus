// Package connector
// File: spec.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package connector

// GeneratedCommandSpec describes a synthesized tool command template.
type GeneratedCommandSpec struct {
	Name          string         `json:"name,omitempty"`
	Description   string         `json:"description,omitempty"`
	Command       string         `json:"command"`
	ReadOnly      bool           `json:"read_only"`
	BackupCommand string         `json:"backup_command,omitempty"`
	Parameters    map[string]any `json:"parameters,omitempty"`
	Required      []string       `json:"required,omitempty"`
}

