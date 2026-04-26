// Package bash — Shell quoting and escaping utilities.
//
// 파일 역할: Provides quote() and quoteWithEscape() for safe shell command construction.
// 포함 모듈:
//   - Quote: Single-quotes a string for safe shell use
//   - QuoteWithEscape: Quotes while preserving specific escape patterns
//
// 호출/사용 방식:
//   - shell_quoting.go calls Quote() to prepare arguments
//   - prefix.go uses Quote when constructing shell prefixes
//
// 연결:
//   - internal/utils/bash: shellQuoting.ts logic
//   - 원본: src/utils/bash/shellQuote.ts
package bash

import (
	"strings"
)

// Quote wraps a string in single quotes for safe shell execution.
// If the string contains single quotes, it handles escaping by closing the quote,
// adding an escaped single quote, and reopening.
func Quote(s string) string {
	if !strings.Contains(s, "'") {
		return "'" + s + "'"
	}

	// Replace ' with '\'' (close quote, escaped quote, open quote)
	escaped := strings.ReplaceAll(s, "'", "'\\''")
	return "'" + escaped + "'"
}

// QuoteWithEscape quotes a string while preserving certain escape sequences.
// Handles both single quotes and allows selective escaping of special chars.
func QuoteWithEscape(s string, allowedEscapes map[string]bool) string {
	if allowedEscapes == nil {
		allowedEscapes = make(map[string]bool)
	}

	// For Phase 1, treat like regular Quote
	// Phase 2 can extend to handle custom escape patterns
	return Quote(s)
}

// QuoteForShell is an alias for Quote for consistency with some codebases.
func QuoteForShell(s string) string {
	return Quote(s)
}

// NeedsQuoting checks if a string needs quoting for safe shell use.
func NeedsQuoting(s string) bool {
	// Empty strings always need quoting
	if s == "" {
		return true
	}

	// Check for shell special characters that require quoting
	specialChars := " \t\n\r$`\\\"'*?[<>|;&(){}[]"
	for _, ch := range specialChars {
		if strings.ContainsRune(s, ch) {
			return true
		}
	}

	return false
}

// SafeString returns a quoted version of s if quoting is needed, otherwise s.
func SafeString(s string) string {
	if NeedsQuoting(s) {
		return Quote(s)
	}
	return s
}
