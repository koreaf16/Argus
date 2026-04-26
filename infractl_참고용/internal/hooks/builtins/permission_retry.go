// Package builtins
// File: permission_retry.go
// Description: PostToolUse builtin hook for one-shot sudo retry rewrites.
// Responsibility: Detect permission denied shell errors and suggest become_method.

package builtins

import (
	"regexp"
	"strings"
)

var (
	permissionDeniedRe = regexp.MustCompile(`(?i)permission\s+denied`)
	ownerEqualsRe      = regexp.MustCompile(`owner=([A-Za-z0-9_.-]+)`)
	ownedByRe          = regexp.MustCompile(`owned by\s+([A-Za-z0-9_.-]+):`)
)

// PermissionRetryHook proposes a single sudo rewrite after shell permission failure.
type PermissionRetryHook struct{}

func (PermissionRetryHook) Name() string { return "permission_retry" }

// Run returns (output, true) when the hook matched and produced a decision.
func (PermissionRetryHook) Run(input RuntimeInput) (RuntimeOutput, bool) {
	if !strings.EqualFold(strings.TrimSpace(input.Event), "PostToolUse") {
		return RuntimeOutput{}, false
	}
	if !strings.EqualFold(strings.TrimSpace(input.Tool), "shell_exec") {
		return RuntimeOutput{}, false
	}
	isError, _ := input.Output["is_error"].(bool)
	if !isError {
		return RuntimeOutput{}, false
	}
	if hasBecomeMethod(input.Input) {
		return RuntimeOutput{}, false
	}

	stderr := extractErrorText(input.Output)
	if !permissionDeniedRe.MatchString(stderr) {
		return RuntimeOutput{}, false
	}

	owner := inferOwnerUser(stderr)
	rewrite := mergeInput(input.Input, map[string]any{
		"become_method": "sudo",
		"become_user":   owner,
	})

	return RuntimeOutput{
		Decision:      "ask",
		SystemMessage: "Permission denied. Retry with sudo?",
		NewInput:      rewrite,
	}, true
}

func hasBecomeMethod(input map[string]any) bool {
	method, _ := input["become_method"].(string)
	return strings.TrimSpace(method) != ""
}

func extractErrorText(output map[string]any) string {
	if stderr, ok := output["stderr"].(string); ok && strings.TrimSpace(stderr) != "" {
		return stderr
	}
	if text, ok := output["output"].(string); ok {
		return text
	}
	return ""
}

func inferOwnerUser(stderr string) string {
	if m := ownerEqualsRe.FindStringSubmatch(stderr); len(m) == 2 {
		owner := strings.TrimSpace(m[1])
		if owner != "" {
			return owner
		}
	}
	if m := ownedByRe.FindStringSubmatch(stderr); len(m) == 2 {
		owner := strings.TrimSpace(m[1])
		if owner != "" {
			return owner
		}
	}
	return "root"
}

func mergeInput(base map[string]any, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}
