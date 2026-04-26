// Package executor
// File: quote.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package executor

import "strings"

// QuotePOSIX returns a single-quoted POSIX shell literal.
func QuotePOSIX(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

// QuotePowerShell returns a single-quoted PowerShell literal.
func QuotePowerShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

