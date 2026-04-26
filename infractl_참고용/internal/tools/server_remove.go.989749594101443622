// Package tools
// File: server_remove.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/store"
)

type serverConnectorCleanup interface {
	DeactivateServer(ctx context.Context, serverName string) error
}

// ServerRemoveTool removes a registered SSH server and its runtime bindings.
type ServerRemoveTool struct {
	Store               store.ServerStore
	Manager             *executor.Manager
	ConnectorCleanup    serverConnectorCleanup
	ActiveServer        func() *store.Server
	OnActiveServerClear func()
	ToolName            string
}

func (t *ServerRemoveTool) Name() string {
	if strings.TrimSpace(t.ToolName) != "" {
		return t.ToolName
	}
	return "server_remove"
}

func (t *ServerRemoveTool) Description() string {
	return "Remove a registered SSH workspace by name"
}

func (t *ServerRemoveTool) IsReadOnly() bool { return false }
func (t *ServerRemoveTool) IsEnabled() bool  { return true }

func (t *ServerRemoveTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "The workspace alias to remove",
			},
		},
		"required": []string{"name"},
	}
}

func (t *ServerRemoveTool) Execute(ctx context.Context, args map[string]interface{}, _ executor.Executor) (ToolOutcome, error) {
	name, err := argString(args, "name", true)
	if err != nil {
		return ToolOutcome{Content: fmt.Sprintf("Error: %s", err), Success: true}, nil
	}

	if t.ConnectorCleanup != nil {
		if err := t.ConnectorCleanup.DeactivateServer(ctx, name); err != nil {
			return ToolOutcome{Content: fmt.Sprintf("Failed to detach connectors for workspace '%s': %s", name, err), Success: true}, nil
		}
	}

	if t.Manager != nil && t.Manager.Has(name) {
		if err := t.Manager.Remove(name); err != nil {
			return ToolOutcome{Content: fmt.Sprintf("Failed to detach runtime executor for workspace '%s': %s", name, err), Success: true}, nil
		}
	}

	if err := t.Store.Remove(ctx, name); err != nil {
		return ToolOutcome{Content: fmt.Sprintf("Failed to remove workspace '%s': %s", name, err), Success: true}, nil
	}

	if active := t.snapshotActiveServer(); active != nil && strings.EqualFold(active.Name, name) && t.OnActiveServerClear != nil {
		t.OnActiveServerClear()
	}

	return ToolOutcome{Content: fmt.Sprintf("Workspace '%s' removed.", name), Success: true}, nil
}

func (t *ServerRemoveTool) snapshotActiveServer() *store.Server {
	if t.ActiveServer == nil {
		return nil
	}
	srv := t.ActiveServer()
	if srv == nil {
		return nil
	}
	cp := *srv
	return &cp
}
