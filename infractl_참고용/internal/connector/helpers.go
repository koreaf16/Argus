// Package connector
// File: helpers.go
// Description: shared helper functions for connector package
// Responsibility: provide output formatting and safe tool-name normalization

package connector

import (
	"fmt"
	"regexp"
	"strings"
)

var invalidConnectorToolName = regexp.MustCompile(`[^a-z0-9_]+`)

func sanitizeToolName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, "-", "_")
	s = invalidConnectorToolName.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if s == "" {
		return "tool"
	}
	return s
}

func formatOutput(stdout, stderr string, exitCode int) string {
	out := strings.TrimSpace(stdout)
	errOut := strings.TrimSpace(stderr)
	if exitCode != 0 && errOut != "" {
		if out == "" {
			return fmt.Sprintf("[ExitCode: %d]\n%s", exitCode, errOut)
		}
		return fmt.Sprintf("[ExitCode: %d]\n%s\n[Stderr]\n%s", exitCode, out, errOut)
	}
	if out == "" {
		return "(no output)"
	}
	return out
}
