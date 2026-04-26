// Package tools
// File: service_status.go
// Description: 시스템 서비스 상태 조회 전용 도구
// Responsibility: systemctl, sc query 등 서비스 관리 명령을 단일 도구로 대체

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

// ServiceStatusTool은 대상 시스템의 서비스 상태를 조회한다.
type ServiceStatusTool struct{}

func (t *ServiceStatusTool) Name() string { return "service_status" }

func (t *ServiceStatusTool) Description() string {
	return "Check the status of system services (systemd, Windows Service, init.d).\n" +
		"Use this INSTEAD of shell_exec with 'systemctl status', 'sc query', 'service --status-all'.\n" +
		"Supports listing all services or checking a specific service."
}

func (t *ServiceStatusTool) IsReadOnly() bool     { return true }
func (t *ServiceStatusTool) IsEnabled() bool      { return true }

func (t *ServiceStatusTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"service": map[string]interface{}{
				"type":        "string",
				"description": "Specific service name to check. Omit to list all services.",
			},
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action: status (detail), list (all services), is-active (running check). Default: status.",
				"enum":        []string{"status", "list", "is-active"},
				"default":     "status",
			},
			"target": map[string]interface{}{
				"type":        "string",
				"description": "Target server name. Omit to use the active workspace. Use 'localhost' ONLY for the local controller machine.",
			},
		},
	}
}

func (t *ServiceStatusTool) Execute(ctx context.Context, args map[string]interface{}, exec executor.Executor) (ToolOutcome, error) {
	service, _ := argString(args, "service", false)
	action, _ := argString(args, "action", false)
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		action = "status"
	}

	platform := executor.CommandPlatform(exec)
	cmd := buildServiceCommand(service, action, platform)
	if cmd == "" {
		return ToolOutcome{Content: "unsupported action: " + action, Success: false, ErrorMessage: "unsupported action: " + action}, nil
	}

	result, err := exec.Execute(ctx, cmd)
	if err != nil {
		return ToolOutcome{Content: fmt.Sprintf("execution failed: %s", err), Success: true}, nil
	}
	if result.ExitCode != 0 && result.Stdout == "" {
		return ToolOutcome{Content: fmt.Sprintf("error (exit %d):\n%s", result.ExitCode, result.Stderr), Success: true}, nil
	}
	out := strings.TrimSpace(result.Stdout)
	if result.Stderr != "" {
		out += "\n[stderr]\n" + strings.TrimSpace(result.Stderr)
	}
	return ToolOutcome{Content: out, Success: true}, nil
}

func buildServiceCommand(service, action string, platform executor.Platform) string {
	switch platform {
	case executor.PlatformWindows:
		return buildServiceWindows(service, action)
	default:
		return buildServiceLinux(service, action)
	}
}

func buildServiceLinux(service, action string) string {
	switch action {
	case "list":
		return "systemctl list-units --type=service --no-pager 2>/dev/null || service --status-all 2>/dev/null"
	case "is-active":
		if service == "" {
			return "systemctl list-units --type=service --state=running --no-pager"
		}
		return fmt.Sprintf("systemctl is-active %s", executor.QuotePOSIX(service))
	case "status":
		if service == "" {
			return "systemctl list-units --type=service --no-pager 2>/dev/null || service --status-all 2>/dev/null"
		}
		return fmt.Sprintf("systemctl status %s --no-pager", executor.QuotePOSIX(service))
	default:
		return ""
	}
}

func buildServiceWindows(service, action string) string {
	switch action {
	case "list":
		return "sc query state= all"
	case "is-active":
		if service == "" {
			return "sc query state= active"
		}
		return fmt.Sprintf("sc query %q", service)
	case "status":
		if service == "" {
			return "sc query state= all"
		}
		return fmt.Sprintf("sc query %q", service)
	default:
		return ""
	}
}
