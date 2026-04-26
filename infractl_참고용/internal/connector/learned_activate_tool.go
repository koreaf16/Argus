// Package connector
// File: learned_activate_tool.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package connector

import (
	"github.com/yourorg/infractl/internal/tools"
	"context"
	"fmt"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/store"
)

// LearnedActivateTool recreates a generic connector from a learned system entry.
type LearnedActivateTool struct {
	Manager *Manager
	Store   store.LearnedSystemStore
}

func (t *LearnedActivateTool) Name() string { return "learned_connector_activate" }

func (t *LearnedActivateTool) Description() string {
	return "Activate a generic connector from a previously saved learned system. Use this when the system is unsupported by built-in connectors but you already saved its commands with save_learned_system."
}

func (t *LearnedActivateTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"service_type": map[string]interface{}{
				"type":        "string",
				"description": "Saved learned system type",
			},
			"server_name": map[string]interface{}{
				"type":        "string",
				"description": "Server where the learned system was saved",
			},
			"service_name": map[string]interface{}{
				"type":        "string",
				"description": "Optional connector display name. Defaults to service_type.",
			},
			"save_mode": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"permanent", "session", "none"},
				"description": "Connector save mode. Default is session.",
			},
		},
		"required": []string{"service_type", "server_name"},
	}
}

func (t *LearnedActivateTool) IsReadOnly() bool { return false }
func (t *LearnedActivateTool) IsEnabled() bool  { return true }

func (t *LearnedActivateTool) Execute(ctx context.Context, args map[string]interface{}, _ executor.Executor) (tools.ToolOutcome, error) {
	serviceType, _ := args["service_type"].(string)
	serverName, _ := args["server_name"].(string)
	serviceName, _ := args["service_name"].(string)
	saveModeStr, _ := args["save_mode"].(string)
	if serviceType == "" || serverName == "" {
		return tools.ToolOutcome{Content: "service_type and server_name are required", Success: true}, nil
	}
	if saveModeStr == "" {
		saveModeStr = string(SaveSession)
	}
	if serviceName == "" {
		serviceName = serviceType
	}

	sys, err := t.Store.GetLearnedSystem(ctx, serviceType, serverName)
	if err != nil {
		return tools.ToolOutcome{}, fmt.Errorf("get learned system: %w", err)
	}

	info := ServiceInfo{
		ServerName:  serverName,
		ServiceType: "generic",
		Name:        serviceName,
		Details: map[string]string{
			"commands_json": sys.Commands,
			"service_type":  sys.ServiceType,
			"cli_path":      sys.CLIPath,
			"config_path":   sys.ConfigPath,
			"log_path":      sys.LogPath,
		},
	}
	if err := t.Manager.Activate(ctx, info, Credentials{}, SaveMode(saveModeStr), nil); err != nil {
		return tools.ToolOutcome{}, fmt.Errorf("activate learned connector: %w", err)
	}
	return tools.ToolOutcome{Content: fmt.Sprintf("Activated learned generic connector for %s @ %s", serviceType, serverName), Success: true}, nil
}

