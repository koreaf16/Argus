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
	return "워크스페이스의 종합적인 환경 정보(OS, 커널, 셸, 사용자, 가동 시간, 메모리, 디스크, 리스닝 포트, 실행 중인 서비스, 주요 프로세스, Docker 컨테이너 등)를 수집합니다. 로컬 및 원격 SSH 워크스페이스 모두에서 작동하며, 결과는 자동으로 캐시되어 시스템 컨텍스트에 삽입됩니다."
}

func (t *ServerInspectTool) InputSchema() tool.ToolInputJSONSchema {
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

func (t *ServerInspectTool) IsReadOnly() bool {
	return true
}

func (t *ServerInspectTool) MaxResultSizeChars() int {
	return 32000
}

func (t *ServerInspectTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
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
		alias, _, err := tool.ResolveExecutionRoleServer(ctx, req.Server, req.Role, req.Channel, "server_inspect")
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}

		snap, err := ctx.Workspace.RunInspect(ctx.Context, alias)
		if err != nil {
			events <- tool.NewErrorEvent(fmt.Errorf("%s 환경 정보 확인 실패: %w", alias, err))
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
			Message:  "워크스페이스 관리자를 사용할 수 없습니다",
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
