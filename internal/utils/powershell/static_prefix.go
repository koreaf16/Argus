// Package powershell — Static PowerShell command prefix extraction.
//
// 파일 역할: Extracts and validates command prefixes in PowerShell without LLM.
//
//	Handles execution policies, profile loading, and static analysis.
//
// 포함 모듈:
//   - GetPowerShellCommandPrefix: Extracts leading prefix (static, no LLM)
//   - ValidatePowerShellPrefix: Validates prefix safety
//
// 호출/사용 방식:
//   - tools/powershell uses this for command classification
//   - tools/bash may need to convert bash commands to PowerShell equivalents
//
// 연결:
//   - internal/utils/powershell: parser.go, dangerous_cmdlets.go
//   - 원본: src/utils/powershell/ static prefix logic
package powershell

import (
	"regexp"
	"strings"
)

// PowerShellPrefix describes a PowerShell command prefix structure.
type PowerShellPrefix struct {
	Raw            string
	ExecutionFlags []string // -NoProfile, -NoExit, -WindowStyle
	Bypass         bool     // -ExecutionPolicy Bypass
	NoInteractive  bool     // -NonInteractive
	Command        string   // -Command or -File
	Arguments      []string // -ArgumentList values
}

// GetPowerShellCommandPrefix extracts the leading prefix from a PowerShell command (static analysis).
// Returns the prefix string and remaining command text.
func GetPowerShellCommandPrefix(command string) (prefix, remaining string) {
	trimmed := strings.TrimSpace(command)

	// PowerShell common prefix patterns:
	// powershell -NoProfile -NoExit -Command "..."
	// pwsh -ExecutionPolicy Bypass -File script.ps1

	prefixKeywords := []struct {
		keyword string
		hasArg  bool
	}{
		{"-NoProfile", false},
		{"-NoExit", false},
		{"-NonInteractive", false},
		{"-NonInteractive", false},
		{"-ExecutionPolicy", true},
		{"-WindowStyle", true},
		{"-EncodedCommand", true},
		{"-EncodedArguments", true},
		{"-ArgumentList", true},
		{"-Command", true},
		{"-File", true},
	}

	// Simple extraction: look for known flags
	for _, kw := range prefixKeywords {
		if strings.Contains(trimmed, kw.keyword) {
			// Extract up to and including the keyword (and its argument if applicable)
			idx := strings.Index(trimmed, kw.keyword)
			if idx != -1 {
				// For Phase 1, just mark that a prefix exists
				prefix += kw.keyword
				if kw.hasArg {
					prefix += " <arg>"
				}
				prefix += " "
			}
		}
	}

	if prefix != "" {
		prefix = strings.TrimSpace(prefix)
		remaining = trimmed
	} else {
		remaining = trimmed
	}

	return prefix, remaining
}

// ValidatePowerShellPrefix validates that a PowerShell prefix is safe.
// Returns true if the prefix appears safe; false if it contains dangerous flags.
func ValidatePowerShellPrefix(prefixStr string) bool {
	lowered := strings.ToLower(prefixStr)

	// Check for dangerous combinations
	dangerousPatterns := []string{
		"-executionpolicy bypass",
		"-ep bypass",
		"-noprofile",      // Can hide security policies
		"-encodedcommand", // Could hide malicious code
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(lowered, pattern) {
			return false
		}
	}

	return true
}

// ParsePowerShellPrefix parses a PowerShell prefix into structured components.
func ParsePowerShellPrefix(prefixStr string) *PowerShellPrefix {
	result := &PowerShellPrefix{
		Raw:            prefixStr,
		ExecutionFlags: make([]string, 0),
		Arguments:      make([]string, 0),
	}

	trimmed := strings.ToLower(prefixStr)

	// Check for execution policy bypass
	if strings.Contains(trimmed, "-executionpolicy") && strings.Contains(trimmed, "bypass") {
		result.Bypass = true
	}

	// Check for non-interactive
	if strings.Contains(trimmed, "-noninteractive") {
		result.NoInteractive = true
	}

	// Check for execution flags
	flags := []string{"-noprofile", "-noexit", "-windowstyle"}
	for _, flag := range flags {
		if strings.Contains(trimmed, flag) {
			result.ExecutionFlags = append(result.ExecutionFlags, flag)
		}
	}

	// Check for command vs file
	if strings.Contains(trimmed, "-command") || strings.Contains(trimmed, "-c") {
		result.Command = "Command"
	} else if strings.Contains(trimmed, "-file") || strings.Contains(trimmed, "-f") {
		result.Command = "File"
	}

	return result
}

// FormatPowerShellPrefixCommand reconstructs a PowerShell command with prefix.
func FormatPowerShellPrefixCommand(prefix, command string) string {
	prefixStr := strings.TrimSpace(prefix)
	cmdStr := strings.TrimSpace(command)

	if prefixStr == "" {
		return cmdStr
	}

	return prefixStr + " " + cmdStr
}

// ConvertBashToPowerShell converts common bash commands to PowerShell equivalents.
// Phase 1: Simple mappings for common commands.
func ConvertBashToPowerShell(bashCmd string) string {
	mappings := map[string]string{
		"ls":       "Get-ChildItem",
		"cat":      "Get-Content",
		"rm":       "Remove-Item",
		"mkdir":    "New-Item -ItemType Directory",
		"touch":    "New-Item -ItemType File",
		"echo":     "Write-Output",
		"pwd":      "Get-Location",
		"cd":       "Set-Location",
		"cp":       "Copy-Item",
		"mv":       "Move-Item",
		"find":     "Get-ChildItem -Recurse",
		"grep":     "Select-String",
		"sort":     "Sort-Object",
		"uniq":     "Get-Unique",
		"wc":       "Measure-Object",
		"head":     "Select-Object -First",
		"tail":     "Select-Object -Last",
		"date":     "Get-Date",
		"sleep":    "Start-Sleep",
		"whoami":   "whoami",
		"hostname": "hostname",
	}

	lower := strings.ToLower(bashCmd)
	for bash, ps := range mappings {
		if strings.HasPrefix(lower, bash) {
			// Simple replacement (Phase 1)
			return strings.Replace(bashCmd, bash, ps, 1)
		}
	}

	return bashCmd
}

// IsValidPowerShellCommand checks if a command looks like valid PowerShell syntax.
// Phase 1: Very basic validation.
func IsValidPowerShellCommand(command string) bool {
	if strings.TrimSpace(command) == "" {
		return false
	}

	// Check for matching braces
	openBraces := strings.Count(command, "{") + strings.Count(command, "(") + strings.Count(command, "[")
	closeBraces := strings.Count(command, "}") + strings.Count(command, ")") + strings.Count(command, "]")

	if openBraces != closeBraces {
		return false
	}

	return true
}

// ExtractScriptPath extracts the file path from a PowerShell -File argument.
func ExtractScriptPath(command string) string {
	re := regexp.MustCompile(`-File\s+(\S+)`)
	matches := re.FindStringSubmatch(command)
	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}
