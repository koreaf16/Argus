package serverinventory

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/koreaf16/argus/internal/services/inventory"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
)

type ServerInventoryTool struct{}

func NewServerInventoryTool() *ServerInventoryTool {
	return &ServerInventoryTool{}
}

func (t *ServerInventoryTool) Name() string { return "server_inventory" }

func (t *ServerInventoryTool) Description(_ tool.Context) string {
	return "원격 서버의 시스템 인벤토리를 수집합니다. Docker 컨테이너, Kubernetes 파드, LLM 서빙(vLLM/Ollama 등) 프로세스의 호스팅 방식(k8s pod/docker/native)을 파악하여 LLM 컨텍스트에 주입합니다. 서버 접속 직후 자동 실행되며, 결과는 5분간 캐시됩니다."
}

func (t *ServerInventoryTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"server": map[string]any{
				"type":        "string",
				"description": "워크스페이스 별칭. 기본값은 활성 워크스페이스입니다.",
			},
			"force": map[string]any{
				"type":        "boolean",
				"description": "true이면 캐시를 무시하고 재수집합니다.",
			},
		},
	}
}

func (t *ServerInventoryTool) IsReadOnly() bool       { return true }
func (t *ServerInventoryTool) MaxResultSizeChars() int { return 8000 }

func (t *ServerInventoryTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	events := make(chan tool.ToolEvent, 1)
	if ctx.Workspace == nil {
		return nil, fmt.Errorf("워크스페이스 관리자를 사용할 수 없습니다")
	}

	var req struct {
		Server string `json:"server"`
		Force  bool   `json:"force"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	alias := strings.TrimSpace(req.Server)
	if alias == "" {
		alias = ctx.Workspace.ActiveAlias()
	}

	go func() {
		defer close(events)
		snap, err := ctx.Workspace.RescanInventory(ctx.Context, alias)
		if err != nil {
			events <- tool.NewErrorEvent(fmt.Errorf("인벤토리 수집 실패: %w", err))
			return
		}
		events <- tool.NewOutputEvent(inventory.FormatSummaryForPrompt(snap))
		events <- tool.NewDoneEvent()
	}()

	return events, nil
}

func (t *ServerInventoryTool) CheckPermission(_ tool.Context, _ json.RawMessage) (tool.PermissionResult, error) {
	return tool.PermissionResult{Behavior: types.BehaviorAllow}, nil
}
