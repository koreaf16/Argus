package serverconnect

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/koreaf16/argus/internal/services/workspace"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
)

type ServerConnectTool struct{}

func NewServerConnectTool() *ServerConnectTool {
	return &ServerConnectTool{}
}

func (t *ServerConnectTool) Name() string {
	return "server_connect"
}

func (t *ServerConnectTool) Description(ctx tool.Context) string {
	return "Establish a connection to a remote server. Supports auto-registration if ad-hoc host/user are provided."
}

func (t *ServerConnectTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"server": map[string]any{
				"type":        "string",
				"description": "Server alias (e.g., 'sandbox'). If host/user are provided, it will auto-register this alias.",
			},
			"host": map[string]any{
				"type":        "string",
				"description": "Remote IP or hostname.",
			},
			"user": map[string]any{
				"type":        "string",
				"description": "SSH username.",
			},
			"port": map[string]any{
				"type":        "integer",
				"description": "SSH port (default 22).",
			},
			"password": map[string]any{
				"type":        "string",
				"description": "Optional password.",
			},
		},
		"required": []string{"server"},
	}
}

func (t *ServerConnectTool) IsReadOnly() bool {
	return true
}

func (t *ServerConnectTool) MaxResultSizeChars() int {
	return 2048
}

func (t *ServerConnectTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	events := make(chan tool.ToolEvent, 1)

	var req struct {
		Server   string `json:"server"`
		Host     string `json:"host"`
		User     string `json:"user"`
		Port     int    `json:"port"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	go func() {
		defer close(events)
		alias := strings.TrimSpace(req.Server)
		if alias == "" {
			events <- tool.NewErrorEvent(fmt.Errorf("server alias is required"))
			return
		}

		if ctx.Workspace == nil {
			events <- tool.NewErrorEvent(fmt.Errorf("workspace manager is unavailable"))
			return
		}

		// [자동 등록 로직] 에일리어스가 없는데 상세 정보가 있다면 즉시 등록
		if strings.TrimSpace(req.Host) != "" {
			user := req.User
			if user == "" {
				user = "root"
			}
			port := req.Port
			if port == 0 {
				port = 22
			}
			ctx.Workspace.UpsertServer(workspace.ServerEntry{
				Alias: alias,
				Host:  req.Host,
				Port:  port,
				User:  user,
				Kind:  workspace.ServerKindSSH,
				Auth:  workspace.ServerAuth{AllowPassword: true},
			})
		}

		// 패스워드 주입
		if strings.TrimSpace(req.Password) != "" {
			ctx.Workspace.SetPassword(alias, "ssh", req.Password)
		}

		// [자율성 보강] 에일리어스가 없는데 IP 형식이라면 스스로 등록 시도
		if strings.Contains(alias, ".") || strings.HasPrefix(alias, "192.") {
			ctx.Workspace.UpsertServer(workspace.ServerEntry{
				Alias: alias,
				Host:  alias,
				Port:  22,
				User:  "sandbox", // 기본 관리자 계정 시도
				Kind:  workspace.ServerKindSSH,
				Auth:  workspace.ServerAuth{AllowPassword: true},
			})
		}

		resolvedAlias, err := ctx.Workspace.ConnectAndActivate(alias)
		if err != nil {
			events <- tool.NewErrorEvent(fmt.Errorf("failed to connect: %w", err))
			return
		}

		if ctx.State != nil {
			ctx.State.SetActiveWorkspace(ctx.Workspace.ActiveAlias())
		}

		events <- tool.NewOutputEvent(fmt.Sprintf("Connected to %s. Inspecting...", resolvedAlias))
		snap, err := ctx.Workspace.RunInspect(ctx.Context, resolvedAlias)
		if err == nil {
			events <- tool.NewOutputEvent(workspace.FormatInspectSummary(snap))
		}
		events <- tool.NewDoneEvent()
	}()

	return events, nil
}

func (t *ServerConnectTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	return tool.PermissionResult{Behavior: types.BehaviorAllow}, nil
}
