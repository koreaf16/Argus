// Package tools
// File: save_learned_system.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package tools

import (
	"context"
	"fmt"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/rag"
	"github.com/yourorg/infractl/internal/store"
)

// SaveLearnedSystemTool stores discovered service metadata for later reuse.
type SaveLearnedSystemTool struct {
	Store  store.LearnedSystemStore
	Memory *rag.MemoryService
}

func (t *SaveLearnedSystemTool) Name() string         { return "save_learned_system" }
func (t *SaveLearnedSystemTool) IsReadOnly() bool     { return false }
func (t *SaveLearnedSystemTool) IsEnabled() bool      { return true }

func (t *SaveLearnedSystemTool) Description() string {
	return "Save learned system information after exploring an unknown service. Saved systems are indexed into internal memory and can be turned back into a generic connector later."
}

func (t *SaveLearnedSystemTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"service_type": map[string]interface{}{
				"type":        "string",
				"description": "Service type (e.g. 'kafka', 'redis', 'nginx', 'elasticsearch')",
			},
			"server_name": map[string]interface{}{
				"type":        "string",
				"description": "Server name where this system was discovered (use 'localhost' for local)",
			},
			"cli_path": map[string]interface{}{
				"type":        "string",
				"description": "Path to CLI binaries",
			},
			"config_path": map[string]interface{}{
				"type":        "string",
				"description": "Path to configuration files",
			},
			"log_path": map[string]interface{}{
				"type":        "string",
				"description": "Path to log files",
			},
			"commands": map[string]interface{}{
				"type": "string",
				"description": `JSON object with learned commands. For state-changing commands, ALWAYS include backup_command. Format:
{
  "status": "kafka-topics.sh --list",
  "restart": {
    "command": "systemctl restart kafka",
    "description": "Kafka ?�시??,
    "read_only": false,
    "backup_command": "cp /etc/kafka/server.properties /etc/kafka/server.properties.bak"
  }
}
For destructive or service-stopping commands, backup_command is REQUIRED.`,
			},
		},
		"required": []string{"service_type", "server_name"},
	}
}

func (t *SaveLearnedSystemTool) Execute(ctx context.Context, args map[string]interface{}, _ executor.Executor) (ToolOutcome, error) {
	serviceType, err := argString(args, "service_type", true)
	if err != nil {
		return ToolOutcome{}, err
	}
	serverName, err := argString(args, "server_name", true)
	if err != nil {
		return ToolOutcome{}, err
	}
	cliPath, _ := argString(args, "cli_path", false)
	configPath, _ := argString(args, "config_path", false)
	logPath, _ := argString(args, "log_path", false)
	commands, _ := argString(args, "commands", false)
	if commands == "" {
		commands = "{}"
	}

	sys := store.LearnedSystem{
		ServiceType: serviceType,
		ServerName:  serverName,
		CLIPath:     cliPath,
		ConfigPath:  configPath,
		LogPath:     logPath,
		Commands:    commands,
	}

	id, err := t.Store.SaveLearnedSystem(ctx, sys)
	if err != nil {
		return ToolOutcome{}, fmt.Errorf("save learned system: %w", err)
	}
	sys.ID = id
	if t.Memory != nil {
		if err := t.Memory.IndexLearnedSystem(ctx, sys); err != nil {
			return ToolOutcome{}, fmt.Errorf("index learned system: %w", err)
		}
	}

	return ToolOutcome{Content: fmt.Sprintf("Saved learned system (ID: %d)\nService: %s @ %s", id, serviceType, serverName), Success: true}, nil
}
