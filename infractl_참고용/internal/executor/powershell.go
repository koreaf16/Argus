// Package executor
// File: powershell.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package executor

import (
	"encoding/base64"
	"strings"
	"unicode/utf16"
)

// PowerShellCommand returns a command string suitable for the target executor.
// Local Windows execution uses the raw script because LocalExecutor already launches PowerShell.
// Remote Windows execution uses EncodedCommand to avoid shell-quoting issues.
func PowerShellCommand(exec Executor, script string) string {
	script = strings.TrimSpace(script)
	if exec == nil || IsLocalTarget(exec.Target()) {
		return script
	}
	return "powershell -NoProfile -NonInteractive -EncodedCommand " + EncodePowerShell(script)
}

// EncodePowerShell returns a base64-encoded UTF-16LE PowerShell payload.
func EncodePowerShell(script string) string {
	encoded := utf16.Encode([]rune(script))
	bytes := make([]byte, 0, len(encoded)*2)
	for _, value := range encoded {
		bytes = append(bytes, byte(value), byte(value>>8))
	}
	return base64.StdEncoding.EncodeToString(bytes)
}

