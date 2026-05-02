package serverinspect

import (
	"encoding/json"
	"fmt"

	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
)

type ServerInspectTool struct{}

func NewServerInspectTool() *ServerInspectTool {
	return &ServerInspectTool{}
}

func (t *ServerInspectTool) Name() string {
	return "server_inspect"
}

func (t *ServerInspectTool) Description(ctx tool.Context) string {
	return "Collect comprehensive environment information for a workspace (OS, kernel, shell, user, uptime, memory, disk, listening ports, running services, top processes, Docker containers). Works on both local and remote SSH workspaces. Results are cached and injected into the system context automatically."
}

func (t *ServerInspectTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"server": map[string]any{
				"type":        "string",
				"description": "Optional workspace alias. Defaults to the active workspace.",
			},
			"role":    map[string]any{"type": "string", "description": "Optional workflow role."},
			"channel": map[string]any{"type": "string", "description": "Optional workflow channel."},
		},
	}
}

func (t *ServerInspectTool) IsReadOnly() bool {
	return true
}

func (t *ServerInspectTool) MaxResultSizeChars() int {
	return 32000
}

func (t *ServerInspectTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	events := make(chan tool.ToolEvent, 2)
	if ctx.Workspace == nil {
		return nil, fmt.Errorf("workspace manager is unavailable")
	}

	var req struct {
		Server  string `json:"server"`
		Role    string `json:"role"`
		Channel string `json:"channel"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	go func() {
		defer close(events)
		alias, _, err := tool.ResolveExecutionRoleServer(ctx, req.Server, req.Role, req.Channel, "server_inspect")
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}

		snap, err := ctx.Workspace.RunInspect(ctx.Context, alias)
		if err != nil {
			events <- tool.NewErrorEvent(fmt.Errorf("inspect failed for %s: %w", alias, err))
			return
		}

		raw, _ := json.MarshalIndent(snap, "", "  ")
		events <- tool.NewOutputEvent(string(raw))
		events <- tool.NewDoneEvent()
	}()

	return events, nil
}

func (t *ServerInspectTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	if ctx.Workspace == nil {
		return tool.PermissionResult{
			Behavior: types.BehaviorDeny,
			Message:  "workspace manager is unavailable",
		}, nil
	}
	rawServer := tool.ExtractStringInput(input, "server")
	rawRole := tool.ExtractStringInput(input, "role")
	rawChannel := tool.ExtractStringInput(input, "channel")
	if _, _, err := tool.ResolveExecutionRoleServer(ctx, rawServer, rawRole, rawChannel, "server_inspect"); err != nil {
		return tool.PermissionResult{
			Behavior: types.BehaviorDeny,
			Message:  err.Error(),
		}, nil
	}
	return tool.DefaultAllowPermission(), nil
}
