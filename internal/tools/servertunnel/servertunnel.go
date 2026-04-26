package servertunnel

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/koreaf16/argus/internal/services/workspace"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
)

type ServerTunnelTool struct{}

func NewServerTunnelTool() *ServerTunnelTool {
	return &ServerTunnelTool{}
}

func (t *ServerTunnelTool) Name() string {
	return "server_tunnel"
}

func (t *ServerTunnelTool) Description(ctx tool.Context) string {
	return "Manage SSH local-forward tunnels on a remote workspace. Actions: open, close, list."
}

func (t *ServerTunnelTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "open | close | list",
			},
			"server": map[string]any{
				"type":        "string",
				"description": "Optional workspace alias. Defaults to active workspace.",
			},
			"local_addr": map[string]any{
				"type":        "string",
				"description": "Local bind address, e.g. 127.0.0.1:15432. Optional for open; default is ephemeral 127.0.0.1:0.",
			},
			"remote_addr": map[string]any{
				"type":        "string",
				"description": "Remote destination from the SSH server perspective, e.g. 127.0.0.1:5432.",
			},
			"tunnel_id": map[string]any{
				"type":        "string",
				"description": "Tunnel ID to close.",
			},
		},
		"required": []string{"action"},
	}
}

func (t *ServerTunnelTool) IsReadOnly() bool {
	return false
}

func (t *ServerTunnelTool) MaxResultSizeChars() int {
	return 100000
}

func (t *ServerTunnelTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	events := make(chan tool.ToolEvent, 2)
	if ctx.Workspace == nil {
		return nil, fmt.Errorf("workspace manager is unavailable")
	}

	var req struct {
		Action     string `json:"action"`
		Server     string `json:"server"`
		LocalAddr  string `json:"local_addr"`
		RemoteAddr string `json:"remote_addr"`
		TunnelID   string `json:"tunnel_id"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	go func() {
		defer close(events)

		action := strings.ToLower(strings.TrimSpace(req.Action))
		if action == "" {
			events <- tool.NewErrorEvent(fmt.Errorf("action is required"))
			return
		}

		alias := tool.ResolveWorkspaceAlias(ctx, req.Server)
		entry, ok := ctx.Workspace.Registry().Get(alias)
		if !ok {
			events <- tool.NewErrorEvent(fmt.Errorf("unknown server alias: %s", alias))
			return
		}
		if entry.Kind != workspace.ServerKindSSH {
			events <- tool.NewErrorEvent(fmt.Errorf("server_tunnel requires an SSH workspace"))
			return
		}

		switch action {
		case "open":
			info, err := ctx.Workspace.OpenTunnel(ctx.Context, alias, req.LocalAddr, req.RemoteAddr)
			if err != nil {
				events <- tool.NewErrorEvent(err)
				return
			}
			raw, _ := json.Marshal(info)
			events <- tool.NewOutputEvent(string(raw))
			events <- tool.NewDoneEvent()
		case "close":
			if strings.TrimSpace(req.TunnelID) == "" {
				events <- tool.NewErrorEvent(fmt.Errorf("tunnel_id is required for close"))
				return
			}
			if err := ctx.Workspace.CloseTunnel(alias, req.TunnelID); err != nil {
				events <- tool.NewErrorEvent(err)
				return
			}
			events <- tool.NewOutputEvent(fmt.Sprintf("closed tunnel %s", req.TunnelID))
			events <- tool.NewDoneEvent()
		case "list":
			items, err := ctx.Workspace.ListTunnels(alias)
			if err != nil {
				events <- tool.NewErrorEvent(err)
				return
			}
			raw, _ := json.Marshal(items)
			events <- tool.NewOutputEvent(string(raw))
			events <- tool.NewDoneEvent()
		default:
			events <- tool.NewErrorEvent(fmt.Errorf("unknown action: %s", action))
		}
	}()

	return events, nil
}

func (t *ServerTunnelTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	if ctx.Workspace == nil {
		return tool.PermissionResult{
			Behavior: types.BehaviorDeny,
			Message:  "workspace manager is unavailable",
		}, nil
	}

	action := strings.ToLower(strings.TrimSpace(tool.ExtractStringInput(input, "action")))
	if action == "" {
		return tool.PermissionResult{
			Behavior: types.BehaviorDeny,
			Message:  "action is required",
		}, nil
	}

	rawServer := strings.TrimSpace(tool.ExtractStringInput(input, "server"))
	alias := tool.ResolveWorkspaceAlias(ctx, rawServer)
	entry, ok := ctx.Workspace.Registry().Get(alias)
	if !ok {
		return tool.PermissionResult{
			Behavior: types.BehaviorDeny,
			Message:  fmt.Sprintf("unknown server alias: %s", alias),
		}, nil
	}
	if entry.Kind != workspace.ServerKindSSH {
		return tool.PermissionResult{
			Behavior: types.BehaviorDeny,
			Message:  "server_tunnel requires an SSH workspace",
		}, nil
	}

	if action == "list" {
		return tool.DefaultAllowPermission(), nil
	}
	return tool.PermissionResult{
		Behavior: types.BehaviorAsk,
		Message:  "opening or closing a tunnel requires approval",
	}, nil
}
