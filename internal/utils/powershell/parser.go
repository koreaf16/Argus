// Package powershell — PowerShell command parsing and tokenization.
//
// 파일 역할: Parses PowerShell commands into tokens for analysis and validation.
//
//	Handles cmdlet extraction, parameter detection, and basic syntax.
//
// 포함 모듈:
//   - ParsePowerShellCommand: Tokenizes a PowerShell command into parts
//   - ExtractCmdlet: Gets the main cmdlet name from a command
//   - ExtractParameters: Extracts parameters and their values
//
// 호출/사용 방식:
//   - tools/powershell validation uses this for command analysis
//   - dangerous_cmdlets.go checks extracted cmdlet names
//
// 연결:
//   - internal/utils/powershell: dangerous_cmdlets.go, static_prefix.go
//   - 원본: src/utils/powershell/ parsing logic
package powershell

import (
	"regexp"
	"strings"
)

// PowerShellToken represents a tokenized component of a PowerShell command.
type PowerShellToken struct {
	Type  string // "cmdlet", "parameter", "value", "pipe", "separator"
	Value string
	Start int
	End   int
}

// ParsePowerShellCommand tokenizes a PowerShell command into components.
// Returns a list of tokens representing the command structure.
func ParsePowerShellCommand(command string) []PowerShellToken {
	var tokens []PowerShellToken

	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return tokens
	}

	// Simple tokenizer: split on spaces, pipes, semicolons, and parameter prefixes
	// Phase 1: Very basic implementation. Full PowerShell parsing is Phase 2.

	// Split by pipe
	pipeRegex := regexp.MustCompile(`\s*\|\s*`)
	pipedCmds := pipeRegex.Split(trimmed, -1)

	offset := 0
	for _, pipeSegment := range pipedCmds {
		if offset > 0 {
			tokens = append(tokens, PowerShellToken{
				Type:  "pipe",
				Value: "|",
				Start: offset,
				End:   offset + 1,
			})
		}

		// Tokenize this segment
		segmentTokens := tokenizeSegment(pipeSegment, offset)
		tokens = append(tokens, segmentTokens...)

		offset += len(pipeSegment) + 2 // +2 for " | "
	}

	return tokens
}

// tokenizeSegment tokenizes a single pipeline segment (command and its args).
func tokenizeSegment(segment string, baseOffset int) []PowerShellToken {
	var tokens []PowerShellToken

	// Split on whitespace
	parts := strings.Fields(segment)

	offset := 0
	for i, part := range parts {
		// First part is the cmdlet
		if i == 0 {
			tokens = append(tokens, PowerShellToken{
				Type:  "cmdlet",
				Value: part,
				Start: baseOffset + offset,
				End:   baseOffset + offset + len(part),
			})
		} else if strings.HasPrefix(part, "-") {
			// This is a parameter
			tokens = append(tokens, PowerShellToken{
				Type:  "parameter",
				Value: part,
				Start: baseOffset + offset,
				End:   baseOffset + offset + len(part),
			})
		} else {
			// This is a value
			tokens = append(tokens, PowerShellToken{
				Type:  "value",
				Value: part,
				Start: baseOffset + offset,
				End:   baseOffset + offset + len(part),
			})
		}

		offset += len(part) + 1 // +1 for space
	}

	return tokens
}

// ExtractCmdlet extracts the main cmdlet name from a PowerShell command.
func ExtractCmdlet(command string) string {
	tokens := ParsePowerShellCommand(command)
	for _, tok := range tokens {
		if tok.Type == "cmdlet" {
			return tok.Value
		}
	}

	// Fallback: take first word
	parts := strings.Fields(strings.TrimSpace(command))
	if len(parts) > 0 {
		return parts[0]
	}

	return ""
}

// ExtractParameters extracts parameters and their values from a command.
func ExtractParameters(command string) map[string]string {
	params := make(map[string]string)
	tokens := ParsePowerShellCommand(command)

	for i, tok := range tokens {
		if tok.Type == "parameter" {
			paramName := tok.Value
			// Look for the next value token
			if i+1 < len(tokens) && tokens[i+1].Type == "value" {
				params[paramName] = tokens[i+1].Value
			} else {
				params[paramName] = ""
			}
		}
	}

	return params
}

// HasParameter checks if a command contains a specific parameter.
func HasParameter(command, param string) bool {
	params := ExtractParameters(command)
	normalizedParam := strings.ToLower(param)

	for p := range params {
		if strings.ToLower(p) == normalizedParam {
			return true
		}
	}

	return false
}

// IsPipedCommand checks if a command contains pipe operators.
func IsPipedCommand(command string) bool {
	return strings.Contains(command, "|")
}

// GetCommandChain returns all piped commands in sequence.
func GetCommandChain(command string) []string {
	var chain []string

	pipeRegex := regexp.MustCompile(`\s*\|\s*`)
	segments := pipeRegex.Split(command, -1)

	for _, seg := range segments {
		trimmed := strings.TrimSpace(seg)
		if trimmed != "" {
			chain = append(chain, trimmed)
		}
	}

	return chain
}
