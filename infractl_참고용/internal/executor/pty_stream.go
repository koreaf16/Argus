// Package executor
// File: pty_stream.go
// Description: PTY-enabled stream execution interface and ANSI stripping utility
// Responsibility: Define PTYStreamExecutor interface for interactive prompt handling

package executor

import (
	"context"
	"regexp"
)

// PTYStreamExecutor extends StreamExecutor with PTY-allocated streaming.
// With PTY, interactive prompts (password, sudo, su) appear in stdout
// instead of /dev/tty, enabling the idle handler to detect and auto-respond.
type PTYStreamExecutor interface {
	StreamExecutor
	ExecuteStreamPTY(ctx context.Context, command string, onLine func(string)) (ExecSession, error)
}

// ansiEscapeRegex matches common ANSI escape sequences from PTY output.
var ansiEscapeRegex = regexp.MustCompile(
	`\x1b\[[0-9;]*[A-Za-z]` + // CSI sequences (colors, cursor, etc.)
		`|\x1b\][^\x07]*\x07` + // OSC sequences (title, etc.)
		`|\x1b\[\?[0-9;]*[hl]`, // DEC private mode set/reset
)

// StripANSI removes ANSI escape sequences from a string.
func StripANSI(s string) string {
	return ansiEscapeRegex.ReplaceAllString(s, "")
}
