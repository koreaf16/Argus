package accountshell

import (
	"encoding/json"
	"fmt"
	"strings"

	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
)

type AccountShellTool struct{}

func NewAccountShellTool() *AccountShellTool {
	return &AccountShellTool{}
}

func (t *AccountShellTool) Name() string {
	return "account_shell"
}

func (t *AccountShellTool) Description(ctx tool.Context) string {
	return "List or close persistent account-scoped shell sessions."
}

func (t *AccountShellTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "list or close",
			},
			"server": map[string]any{
				"type":        "string",
				"description": "Optional workspace alias. Defaults to active workspace.",
			},
			"role":    map[string]any{"type": "string", "description": "Optional workflow role."},
			"channel": map[string]any{"type": "string", "description": "Optional workflow channel."},
			"session_id": map[string]any{
				"type":        "string",
				"description": "Account shell session id to close.",
			},
		},
		"required": []string{"action"},
	}
}

func (t *AccountShellTool) IsReadOnly() bool {
	return false
}

func (t *AccountShellTool) MaxResultSizeChars() int {
	return 100000
}

func (t *AccountShellTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	events := make(chan tool.ToolEvent, 2)
	if ctx.Workspace == nil {
		return nil, fmt.Errorf("workspace manager is unavailable")
	}

	var req struct {
		Action    string `json:"action"`
		Server    string `json:"server"`
		Role      string `json:"role"`
		Channel   string `json:"channel"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	go func() {
		defer close(events)
		action := strings.ToLower(strings.TrimSpace(req.Action))
		alias, _, err := tool.ResolveExecutionRoleServer(ctx, req.Server, req.Role, req.Channel, "account_shell")
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}

		switch action {
		case "list":
			items := ctx.Workspace.ListAccountShells(alias)
			raw, _ := json.Marshal(items)
			events <- tool.NewOutputEvent(string(raw))
			events <- tool.NewDoneEvent()
		case "close":
			if strings.TrimSpace(req.SessionID) == "" {
				events <- tool.NewErrorEvent(fmt.Errorf("session_id is required for close"))
				return
			}
			if err := ctx.Workspace.CloseAccountShell(alias, req.SessionID); err != nil {
				events <- tool.NewErrorEvent(err)
				return
			}
			events <- tool.NewOutputEvent(fmt.Sprintf("closed account shell %s", req.SessionID))
			events <- tool.NewDoneEvent()
		default:
			events <- tool.NewErrorEvent(fmt.Errorf("unknown action: %s", action))
		}
	}()

	return events, nil
}

func (t *AccountShellTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	rawServer := tool.ExtractStringInput(input, "server")
	rawRole := tool.ExtractStringInput(input, "role")
	rawChannel := tool.ExtractStringInput(input, "channel")
	if _, _, err := tool.ResolveExecutionRoleServer(ctx, rawServer, rawRole, rawChannel, "account_shell"); err != nil {
		return tool.PermissionResult{
			Behavior: types.BehaviorDeny,
			Message:  err.Error(),
		}, nil
	}
	action := strings.ToLower(strings.TrimSpace(tool.ExtractStringInput(input, "action")))
	if action == "list" {
		return tool.DefaultAllowPermission(), nil
	}
	return tool.PermissionResult{
		Behavior: types.BehaviorAsk,
		Message:  "closing a persistent account shell requires approval",
	}, nil
}
