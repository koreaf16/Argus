// Package tools
// File: process_list.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package tools

import (
	"context"
	"fmt"

	"github.com/yourorg/infractl/internal/executor"
)

// ProcessListTool lists processes on the target system.
type ProcessListTool struct{}

func (t *ProcessListTool) Name() string { return "process_list" }

func (t *ProcessListTool) Description() string {
	return "List running processes with their resource usage (CPU, memory, PID).\n" +
		"Use for: checking if a service is running, identifying resource-hungry processes.\n" +
		"For service discovery, this is the first step before activating a connector."
}

func (t *ProcessListTool) IsReadOnly() bool { return true }
func (t *ProcessListTool) IsEnabled() bool  { return true }

func (t *ProcessListTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"filter": map[string]interface{}{
				"type":        "string",
				"description": "Filter processes by name (optional)",
			},
			"target": map[string]interface{}{
				"type":        "string",
				"description": "Target server name. Omit to use the active workspace. Use 'localhost' ONLY for the local controller machine.",
			},
		},
	}
}

func (t *ProcessListTool) Execute(ctx context.Context, args map[string]interface{}, exec executor.Executor) (ToolOutcome, error) {
	filter, _ := argString(args, "filter", false)

	cmd := buildProcessListCommand(filter, executor.CommandPlatform(exec))
	result, err := exec.Execute(ctx, cmd)
	if err != nil {
		return ToolOutcome{Content: fmt.Sprintf("Execution failed: %s", err), Success: true}, nil
	}

	if result.ExitCode != 0 {
		// grep exits 1 with no stderr when the filter matches nothing — treat as empty result.
		if result.Stderr == "" && filter != "" {
			return ToolOutcome{Content: fmt.Sprintf("No processes matching '%s' found.", filter), Success: true}, nil
		}
		return ToolOutcome{Content: fmt.Sprintf("Error listing processes (exit %d):\n%s", result.ExitCode, result.Stderr), Success: true}, nil
	}
	if result.Stdout == "" {
		return ToolOutcome{Content: "(no processes found)", Success: true}, nil
	}
	return ToolOutcome{Content: result.Stdout, Success: true}, nil
}

func buildProcessListCommand(filter string, platform executor.Platform) string {
	switch platform {
	case executor.PlatformWindows:
		if filter != "" {
			return fmt.Sprintf("tasklist | findstr /i %q", filter)
		}
		return "tasklist"
	default:
		if filter != "" {
			return fmt.Sprintf("ps aux | grep %s | grep -v grep", executor.QuotePOSIX(filter))
		}
		return "ps aux"
	}
}
