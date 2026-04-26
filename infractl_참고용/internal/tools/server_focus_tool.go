// Package tools
// File: server_focus_tool.go
// Description: server focus tool for selecting the current working server
// Responsibility: manage active server focus and clear the runtime SSH session when requested

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/store"
)

// FocusSelectOption represents one selectable server option in the focus UI.
type FocusSelectOption struct {
	Label       string
	Description string
	Preview     string
}

// ServerFocusTool sets or clears the current active server.
type ServerFocusTool struct {
	Store        store.ServerStore
	Manager      *executor.Manager
	ActiveServer func() *store.Server
	OnChange     func(srv *store.Server)
	SelectFn     func(question string, opts []FocusSelectOption) (int, error)
	ToolName     string
}

var _ Tool = (*ServerFocusTool)(nil)

func (t *ServerFocusTool) Name() string {
	if strings.TrimSpace(t.ToolName) != "" {
		return t.ToolName
	}
	return "server_focus"
}

func (t *ServerFocusTool) Description() string {
	return "Set the current active server. If a server is supplied, it becomes the active target. " +
		"If no server is supplied, the user can choose from the registered server list. " +
		"Passing clear/none/localhost/local clears the active remote server and disconnects the runtime SSH session."
}

func (t *ServerFocusTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"server": map[string]interface{}{
				"type":        "string",
				"description": "Server name to activate.",
			},
			"workspace": map[string]interface{}{
				"type":        "string",
				"description": "Server name to activate (legacy alias). Omit to choose from the registered server list.",
			},
		},
		"required": []string{},
	}
}

func (t *ServerFocusTool) IsReadOnly() bool { return false }
func (t *ServerFocusTool) IsEnabled() bool  { return true }

// Execute sets the active server or clears it.
func (t *ServerFocusTool) Execute(ctx context.Context, args map[string]interface{}, _ executor.Executor) (ToolOutcome, error) {
	serverName, _ := args["server"].(string)
	if strings.TrimSpace(serverName) == "" {
		serverName, _ = args["workspace"].(string)
	}

	servers, err := t.Store.List(ctx)
	if err != nil {
		msg := fmt.Sprintf("server list query failed: %s", err)
		return ToolOutcome{Content: msg, Success: false, ErrorMessage: msg}, nil
	}

	ok := func(msg string) (ToolOutcome, error) {
		return ToolOutcome{Content: msg, Success: true}, nil
	}

	if serverName != "" {
		switch strings.ToLower(strings.TrimSpace(serverName)) {
		case "clear", "none", "localhost", "local":
			active := t.snapshotActiveServer()
			if active != nil && t.Manager != nil && t.Manager.Has(active.Name) {
				if err := t.Manager.Remove(active.Name); err != nil {
					msg := fmt.Sprintf("failed to disconnect server %q: %s", active.Name, err)
					return ToolOutcome{Content: msg, Success: false, ErrorMessage: msg}, nil
				}
			}
			if t.OnChange != nil {
				t.OnChange(nil)
			}
			return ok("Active server cleared. SSH session disconnected. Default target is the local server.")
		}

		for _, s := range servers {
			if strings.EqualFold(s.Name, serverName) {
				srv := s
				if t.OnChange != nil {
					t.OnChange(&srv)
				}
				return ok(fmt.Sprintf("Active server: %s (%s:%d, dir: %s)", s.Name, s.Host, s.Port, s.WorkspaceDir))
			}
		}

		msg := fmt.Sprintf("server %q not found. Use server_list to inspect registered servers.", serverName)
		return ToolOutcome{Content: msg, Success: false, ErrorMessage: msg}, nil
	}

	if len(servers) == 0 {
		return ok("No registered servers. Use server_add first.")
	}

	if len(servers) == 1 {
		srv := servers[0]
		if t.OnChange != nil {
			t.OnChange(&srv)
		}
		return ok(fmt.Sprintf("Active server: %s (%s:%d, dir: %s)", srv.Name, srv.Host, srv.Port, srv.WorkspaceDir))
	}

	if t.SelectFn == nil {
		var list string
		for _, s := range servers {
			list += fmt.Sprintf("  - %s (%s:%d, dir: %s)\n", s.Name, s.Host, s.Port, s.WorkspaceDir)
		}
		return ok(fmt.Sprintf("Multiple servers are registered. Specify server parameter:\n%s", list))
	}

	opts := make([]FocusSelectOption, len(servers))
	for i, s := range servers {
		desc := fmt.Sprintf("%s@%s:%d", s.User, s.Host, s.Port)
		preview := fmt.Sprintf("Host: %s\nPort: %d\nUser: %s\nWorkspace: %s", s.Host, s.Port, s.User, s.WorkspaceDir)
		if s.OS != "" {
			preview += "\nOS: " + s.OS
		}
		if s.EnvProfile != "" {
			preview += "\nEnv: " + s.EnvProfile
		}
		opts[i] = FocusSelectOption{
			Label:       s.Name,
			Description: desc,
			Preview:     preview,
		}
	}

	idx, err := t.SelectFn("Select active server", opts)
	if err != nil || idx < 0 {
		return ok("Selection cancelled.")
	}
	if idx >= len(servers) {
		return ok("Invalid selection.")
	}

	srv := servers[idx]
	if t.OnChange != nil {
		t.OnChange(&srv)
	}
	return ok(fmt.Sprintf("Active server: %s (%s:%d, dir: %s)", srv.Name, srv.Host, srv.Port, srv.WorkspaceDir))
}

func (t *ServerFocusTool) snapshotActiveServer() *store.Server {
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
