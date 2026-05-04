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
	return "지속적인 계정 범위의 셸 세션을 나열하거나 닫습니다."
}

func (t *AccountShellTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "작업 유형 (list 또는 close)",
			},
			"server": map[string]any{
				"type":        "string",
				"description": "선택 사항: 워크스페이스 별칭. 기본값은 활성 워크스페이스입니다.",
			},
			"role":    map[string]any{"type": "string", "description": "선택 사항: 워크플로우 역할."},
			"channel": map[string]any{"type": "string", "description": "선택 사항: 워크플로우 채널."},
			"session_id": map[string]any{
				"type":        "string",
				"description": "닫을 계정 셸 세션 ID.",
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
		return nil, fmt.Errorf("워크스페이스 관리자를 사용할 수 없습니다")
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
				events <- tool.NewErrorEvent(fmt.Errorf("닫기 작업을 위해 session_id가 필요합니다"))
				return
			}
			if err := ctx.Workspace.CloseAccountShell(alias, req.SessionID); err != nil {
				events <- tool.NewErrorEvent(err)
				return
			}
			events <- tool.NewOutputEvent(fmt.Sprintf("계정 셸 %s를 닫았습니다", req.SessionID))
			events <- tool.NewDoneEvent()
		default:
			events <- tool.NewErrorEvent(fmt.Errorf("알 수 없는 작업: %s", action))
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
		Message:  "지속적인 계정 셸을 닫으려면 승인이 필요합니다",
	}, nil
}
