// Package bash — Interactive shell prompt monitoring.
package bash

import (
	"regexp"
	"strings"
)

// PromptType defines the type of interactive prompt detected.
type PromptType string

const (
	PromptPassword PromptType = "password"
	PromptYesNo    PromptType = "yesno"
	PromptUnknown  PromptType = "unknown"
)

// InteractiveMonitor monitors shell output for interactive prompts.
type InteractiveMonitor struct {
	PasswordRegex *regexp.Regexp
	YesnoRegex    *regexp.Regexp
}

// NewInteractiveMonitor creates a new interactive monitor with standard patterns.
func NewInteractiveMonitor() *InteractiveMonitor {
	return &InteractiveMonitor{
		// Claude Code style password patterns
		PasswordRegex: regexp.MustCompile(`(?i)(password|passphrase|verification code|token)[:：]\s*$`),
		YesnoRegex:    regexp.MustCompile(`(?i)\(yes/no\)\??\s*$`),
	}
}

// Detect checks if the given output chunk contains an interactive prompt.
func (m *InteractiveMonitor) Detect(output string) (PromptType, bool) {
	trimmed := strings.TrimSpace(output)
	if m.PasswordRegex.MatchString(trimmed) {
		return PromptPassword, true
	}
	if m.YesnoRegex.MatchString(trimmed) {
		return PromptYesNo, true
	}
	return PromptUnknown, false
}

// IsSshPasswordPrompt specifically checks for SSH password prompts.
func (m *InteractiveMonitor) IsSshPasswordPrompt(output string) bool {
	// e.g., "sandbox@192.168.0.130's password: "
	return strings.Contains(output, "'s password:") || strings.Contains(output, "Password:")
}
