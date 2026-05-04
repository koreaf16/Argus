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
	return "원격 워크스페이스에서 SSH 로컬 포워딩 터널을 관리합니다. 작업: open, close, list."
}

func (t *ServerTunnelTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "수행할 작업: open (열기), close (닫기), list (나열)",
			},
			"server": map[string]any{
				"type":        "string",
				"description": "선택적 워크스페이스 별칭. 기본값은 활성 워크스페이스입니다.",
			},
			"local_addr": map[string]any{
				"type":        "string",
				"description": "로컬 바인드 주소 (예: 127.0.0.1:15432). open 시 선택 사항이며 기본값은 임시 주소인 127.0.0.1:0입니다.",
			},
			"remote_addr": map[string]any{
				"type":        "string",
				"description": "SSH 서버 관점에서의 원격 대상 주소 (예: 127.0.0.1:5432).",
			},
			"tunnel_id": map[string]any{
				"type":        "string",
				"description": "닫을 터널의 ID.",
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
		return nil, fmt.Errorf("워크스페이스 관리자를 사용할 수 없습니다")
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
			events <- tool.NewErrorEvent(fmt.Errorf("작업(action)이 필요합니다"))
			return
		}

		alias := tool.ResolveWorkspaceAlias(ctx, req.Server)
		entry, ok := ctx.Workspace.Registry().Get(alias)
		if !ok {
			events <- tool.NewErrorEvent(fmt.Errorf("알 수 없는 서버 별칭: %s", alias))
			return
		}
		if entry.Kind != workspace.ServerKindSSH {
			events <- tool.NewErrorEvent(fmt.Errorf("server_tunnel은 SSH 워크스페이스가 필요합니다"))
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
				events <- tool.NewErrorEvent(fmt.Errorf("터널을 닫으려면 tunnel_id가 필요합니다"))
				return
			}
			if err := ctx.Workspace.CloseTunnel(alias, req.TunnelID); err != nil {
				events <- tool.NewErrorEvent(err)
				return
			}
			events <- tool.NewOutputEvent(fmt.Sprintf("터널 %s을(를) 닫았습니다", req.TunnelID))
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
			events <- tool.NewErrorEvent(fmt.Errorf("알 수 없는 작업: %s", action))
		}
	}()

	return events, nil
}

func (t *ServerTunnelTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	if ctx.Workspace == nil {
		return tool.PermissionResult{
			Behavior: types.BehaviorDeny,
			Message:  "워크스페이스 관리자를 사용할 수 없습니다",
		}, nil
	}

	action := strings.ToLower(strings.TrimSpace(tool.ExtractStringInput(input, "action")))
	if action == "" {
		return tool.PermissionResult{
			Behavior: types.BehaviorDeny,
			Message:  "작업(action)이 필요합니다",
		}, nil
	}

	rawServer := strings.TrimSpace(tool.ExtractStringInput(input, "server"))
	alias := tool.ResolveWorkspaceAlias(ctx, rawServer)
	entry, ok := ctx.Workspace.Registry().Get(alias)
	if !ok {
		return tool.PermissionResult{
			Behavior: types.BehaviorDeny,
			Message:  fmt.Sprintf("알 수 없는 서버 별칭: %s", alias),
		}, nil
	}
	if entry.Kind != workspace.ServerKindSSH {
		return tool.PermissionResult{
			Behavior: types.BehaviorDeny,
			Message:  "server_tunnel은 SSH 워크스페이스가 필요합니다",
		}, nil
	}

	if action == "list" {
		return tool.DefaultAllowPermission(), nil
	}
	return tool.PermissionResult{
		Behavior: types.BehaviorAsk,
		Message:  "터널을 열거나 닫으려면 승인이 필요합니다",
	}, nil
}
