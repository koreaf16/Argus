package probes

import (
	"context"
	"strings"
	"time"
)

// ProbeExec is a function that runs a bash script and returns its stdout.
type ProbeExec func(ctx context.Context, script string) (string, error)

// runScript runs script via the given ProbeExec with the specified timeout.
func runScript(ctx context.Context, fn ProbeExec, timeout time.Duration, script string) (string, error) {
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return fn(tctx, script)
}

// parseColonKV splits "key: value" or "key=value" lines into (key, value).
func parseColonKV(line string) (key, value string) {
	if idx := strings.Index(line, "="); idx >= 0 {
		return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:])
	}
	if idx := strings.Index(line, ":"); idx >= 0 {
		return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:])
	}
	return strings.TrimSpace(line), ""
}

// firstNonEmptyLine returns the first non-blank line of s.
func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}
