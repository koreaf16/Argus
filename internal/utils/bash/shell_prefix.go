// Package bash — Shell prefix command formatting.
//
// 파일 역할: Formats and extracts shell command prefix (without executing LLM classification).
//
//	Static extraction of leading bash/sh control structures and redirects.
//
// 포함 모듈:
//   - GetShellPrefix: Extracts leading prefix from bash command (static, no LLM)
//   - HasShellPrefix: Checks if command has recognized shell prefix
//   - ParseShellPrefix: Parses prefix structure into components
//
// 호출/사용 방식:
//   - prefix.go calls GetShellPrefix when classifying bash commands
//   - tools/bash calls this to detect command modifiers (nohup, env vars, etc.)
//
// 연결:
//   - internal/utils/bash: prefix.ts logic
//   - 원본: src/utils/bash/shellPrefix.ts
package bash

import (
	"regexp"
	"strings"
)

// ShellPrefix describes a shell command's prefix components.
type ShellPrefix struct {
	Raw        string
	EnvVars    map[string]string
	Prefix     string // e.g., "nohup", "time", "sudo"
	Redirects  []string
	Background bool
	StartTime  string // e.g., "00:00" for at or cron
}

// GetShellPrefix extracts the leading prefix from a bash command (static analysis, no LLM).
// Returns the prefix string and remaining command text.
func GetShellPrefix(command string) (prefix, remaining string) {
	trimmed := strings.TrimSpace(command)

	// Remove leading env vars (VAR=value pattern)
	envRegex := regexp.MustCompile(`^([A-Z_][A-Z0-9_]*=\S+\s+)+`)
	prefixMatch := envRegex.FindString(trimmed)
	if prefixMatch != "" {
		prefix += prefixMatch
		trimmed = strings.TrimSpace(trimmed[len(prefixMatch):])
	}

	// Remove leading bash/sh control keywords and prefixes
	prefixKeywords := []string{
		"nohup",
		"time",
		"sudo",
		"env",
		"command",
		"builtin",
		"exec",
		"eval",
	}

	for _, kw := range prefixKeywords {
		if strings.HasPrefix(trimmed, kw+" ") || trimmed == kw {
			prefix += kw + " "
			trimmed = strings.TrimSpace(trimmed[len(kw):])
			break
		}
	}

	return strings.TrimSpace(prefix), trimmed
}

// HasShellPrefix checks if the command starts with recognized shell modifiers.
func HasShellPrefix(command string) bool {
	prefix, _ := GetShellPrefix(command)
	return prefix != ""
}

// ParseShellPrefix parses a shell prefix into structured components.
func ParseShellPrefix(prefixStr string) *ShellPrefix {
	result := &ShellPrefix{
		Raw:     prefixStr,
		EnvVars: make(map[string]string),
	}

	trimmed := strings.TrimSpace(prefixStr)

	// Extract env vars
	envRegex := regexp.MustCompile(`([A-Z_][A-Z0-9_]*)=(\S+)`)
	matches := envRegex.FindAllStringSubmatch(trimmed, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			result.EnvVars[match[1]] = match[2]
		}
	}

	// Check for background execution (&)
	if strings.HasSuffix(trimmed, "&") {
		result.Background = true
	}

	// Check for known prefixes
	knownPrefixes := []string{"nohup", "time", "sudo", "env", "command"}
	for _, kw := range knownPrefixes {
		if strings.Contains(trimmed, kw) {
			result.Prefix = kw
			break
		}
	}

	return result
}

// FormatShellPrefixCommand reconstructs a shell command with prefix.
// This is used to reformat commands with modified prefixes.
func FormatShellPrefixCommand(prefix, command string) string {
	prefixStr := strings.TrimSpace(prefix)
	cmdStr := strings.TrimSpace(command)

	if prefixStr == "" {
		return cmdStr
	}

	return prefixStr + " " + cmdStr
}
