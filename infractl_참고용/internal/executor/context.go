// Package executor
// File: context.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package executor

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// LocalShellName returns the shell label used for local execution.
// On Linux/macOS, resolves the actual shell available in PATH (bash → sh).
func LocalShellName() string {
	if runtime.GOOS == "windows" {
		return "PowerShell"
	}
	for _, sh := range []string{"bash", "sh"} {
		if _, err := exec.LookPath(sh); err == nil {
			return sh
		}
	}
	return "sh"
}

// ExecutionContextLabel returns a stable human-readable execution target label.
func ExecutionContextLabel(exec Executor) string {
	if exec == nil {
		return fmt.Sprintf("localhost (%s)", LocalShellName())
	}

	target := strings.TrimSpace(exec.Target())
	if IsLocalTarget(target) {
		return fmt.Sprintf("localhost (%s)", LocalShellName())
	}

	shell := ShellNameForExecutor(exec)
	if shell == "" || shell == "ssh" {
		return fmt.Sprintf("%s (ssh)", target)
	}
	return fmt.Sprintf("%s (%s over ssh)", target, shell)
}

