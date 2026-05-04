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
	return "활성 SSH 워크스페이스에서 원격 서버 메트릭(CPU/메모리/디스크/가동 시간/프로세스/GPU)을 수집합니다."
}

func (t *ServerMetricsTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"server": map[string]any{
				"type":        "string",
				"description": "선택적 워크스페이스 별칭. 기본값은 활성 워크스페이스입니다.",
			},
			"role":    map[string]any{"type": "string", "description": "선택적 워크플로우 역할."},
			"channel": map[string]any{"type": "string", "description": "선택적 워크플로우 채널."},
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
		return nil, fmt.Errorf("워크스페이스 관리자를 사용할 수 없습니다")
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
			events <- tool.NewErrorEvent(fmt.Errorf("알 수 없는 서버 별칭: %s", alias))
			return
		}
		if entry.Kind != workspace.ServerKindSSH {
			events <- tool.NewErrorEvent(fmt.Errorf("server_metrics는 SSH 워크스페이스가 필요합니다"))
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
			Message:  "워크스페이스 관리자를 사용할 수 없습니다",
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
			Message:  fmt.Sprintf("알 수 없는 서버 별칭: %s", alias),
		}, nil
	}
	if entry.Kind != workspace.ServerKindSSH {
		return tool.PermissionResult{
			Behavior: types.BehaviorDeny,
			Message:  "server_metrics는 SSH 워크스페이스가 필요합니다",
		}, nil
	}
	return tool.DefaultAllowPermission(), nil
}
