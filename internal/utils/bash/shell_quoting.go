// Package bash — Advanced shell quoting and command rewriting.
//
// 파일 역할: Comprehensive shell quoting for commands, handling redirects, pipes,
//
//	and special shell constructs. Also performs Windows-specific null redirect handling.
//
// 포함 모듈:
//   - QuoteShellCommand: Quotes an entire command with args for shell execution
//   - RewriteWindowsNullRedirect: Converts /dev/null to NUL on Windows
//   - ShouldAddStdinRedirect: Determines if stdin should be redirected from /dev/null
//
// 호출/사용 방식:
//   - shell_command.go uses QuoteShellCommand when preparing bash execution
//   - tools/bash sanitizes commands before execution
//
// 연결:
//   - internal/utils/bash: shellQuoting.ts
//   - internal/utils: shell execution path
//   - 원본: src/utils/bash/shellQuoting.ts
package bash

import (
	"runtime"
	"strings"
)

// QuoteShellCommand quotes a command and its arguments for safe shell execution.
// Handles multiple arguments and respects existing quotes.
func QuoteShellCommand(command string, args ...string) string {
	var parts []string

	// Quote the command itself
	parts = append(parts, Quote(command))

	// Quote each argument
	for _, arg := range args {
		parts = append(parts, Quote(arg))
	}

	return strings.Join(parts, " ")
}

// QuoteShellCommandString quotes a command and space-separated argument string.
func QuoteShellCommandString(command string, argsStr string) string {
	parts := []string{Quote(command)}

	// Simple tokenization of args (respecting quoted strings)
	args := tokenizeArgsSimple(argsStr)
	for _, arg := range args {
		parts = append(parts, Quote(arg))
	}

	return strings.Join(parts, " ")
}

// RewriteWindowsNullRedirect converts /dev/null to NUL on Windows.
// This handles common idioms like `command > /dev/null 2>&1`.
func RewriteWindowsNullRedirect(command string) string {
	if runtime.GOOS != "windows" {
		return command
	}

	// Replace /dev/null with NUL
	result := strings.ReplaceAll(command, "/dev/null", "NUL")
	return result
}

// ShouldAddStdinRedirect determines if stdin should be redirected from /dev/null.
// Returns true if the command does not already specify stdin input.
func ShouldAddStdinRedirect(command string) bool {
	// Check for pipes, input redirects, or here-docs that would provide stdin
	hasInput := strings.Contains(command, "|") ||
		strings.Contains(command, "<") ||
		strings.Contains(command, "<<")

	return !hasInput
}

// tokenizeArgsSimple is a very basic argument tokenizer.
// It splits on spaces but respects single and double quotes.
func tokenizeArgsSimple(argsStr string) []string {
	var result []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	for _, ch := range argsStr {
		if escaped {
			current.WriteRune(ch)
			escaped = false
			continue
		}

		switch ch {
		case '\\':
			escaped = true
			current.WriteRune(ch)
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
			current.WriteRune(ch)
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
			current.WriteRune(ch)
		case ' ', '\t':
			if inSingle || inDouble {
				current.WriteRune(ch)
			} else if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// AddNullStdinRedirect adds `< /dev/null` (or `< NUL` on Windows) to a command if needed.
func AddNullStdinRedirect(command string) string {
	if !ShouldAddStdinRedirect(command) {
		return command
	}

	nullDevice := "/dev/null"
	if runtime.GOOS == "windows" {
		nullDevice = "NUL"
	}

	return command + " < " + nullDevice
}

// PrepareCommandForShell applies all necessary transformations before execution.
func PrepareCommandForShell(command string) string {
	// On Windows, convert /dev/null references
	if runtime.GOOS == "windows" {
		command = RewriteWindowsNullRedirect(command)
	}

	return command
}

// IsRedirectOperator checks if a token is a shell redirect operator.
func IsRedirectOperator(token string) bool {
	redirects := []string{
		">", ">>", "<", "<<<", "2>", "2>>", "&>", "&>>",
		"1>", "1>>", "><", "<>",
	}
	for _, r := range redirects {
		if token == r {
			return true
		}
	}
	return false
}

// IsPipeOperator checks if a token is a pipe operator.
func IsPipeOperator(token string) bool {
	return token == "|" || token == "|&"
}
