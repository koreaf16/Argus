package servermetrics

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/koreaf16/argus/internal/services/workspace"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
)

type ServerMetricsTool struct{}

func NewServerMetricsTool() *ServerMetricsTool {
	return &ServerMetricsTool{}
}

func (t *ServerMetricsTool) Name() string {
	return "server_metrics"
}

func (t *ServerMetricsTool) Description(ctx tool.Context) string {
	return "Collect remote server metrics over the active SSH workspace (CPU/memory/disk/uptime/process/GPU)."
}

func (t *ServerMetricsTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"server": map[string]any{
				"type":        "string",
				"description": "Optional workspace alias. Defaults to active workspace.",
			},
			"role":    map[string]any{"type": "string", "description": "Optional workflow role."},
			"channel": map[string]any{"type": "string", "description": "Optional workflow channel."},
		},
	}
}

func (t *ServerMetricsTool) IsReadOnly() bool {
	return true
}

func (t *ServerMetricsTool) MaxResultSizeChars() int {
	return 100000
}

func (t *ServerMetricsTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
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
		alias, _, err := tool.ResolveExecutionRoleServer(ctx, req.Server, req.Role, req.Channel, "server_metrics")
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		entry, ok := ctx.Workspace.Registry().Get(alias)
		if !ok {
			events <- tool.NewErrorEvent(fmt.Errorf("unknown server alias: %s", alias))
			return
		}
		if entry.Kind != workspace.ServerKindSSH {
			events <- tool.NewErrorEvent(fmt.Errorf("server_metrics requires an SSH workspace"))
			return
		}

		snapshot, err := ctx.Workspace.MetricsSnapshot(ctx.Context, alias)
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		raw, _ := json.Marshal(snapshot)
		events <- tool.NewOutputEvent(string(raw))
		events <- tool.NewDoneEvent()
	}()

	return events, nil
}

func (t *ServerMetricsTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	if ctx.Workspace == nil {
		return tool.PermissionResult{
			Behavior: types.BehaviorDeny,
			Message:  "workspace manager is unavailable",
		}, nil
	}

	rawServer := strings.TrimSpace(tool.ExtractStringInput(input, "server"))
	rawRole := tool.ExtractStringInput(input, "role")
	rawChannel := tool.ExtractStringInput(input, "channel")
	alias, _, err := tool.ResolveExecutionRoleServer(ctx, rawServer, rawRole, rawChannel, "server_metrics")
	if err != nil {
		return tool.PermissionResult{
			Behavior: types.BehaviorDeny,
			Message:  err.Error(),
		}, nil
	}
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
			Message:  "server_metrics requires an SSH workspace",
		}, nil
	}
	return tool.DefaultAllowPermission(), nil
}
