package listmcpresourcestool

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/koreaf16/argus/internal/constants"
	"github.com/koreaf16/argus/internal/services/mcp"
	tool "github.com/koreaf16/argus/internal/tools"
)

type ListMcpResourcesTool struct{}

func NewListMcpResourcesTool() *ListMcpResourcesTool { return &ListMcpResourcesTool{} }

func (t *ListMcpResourcesTool) Name() string { return "list_mcp_resources" }

func (t *ListMcpResourcesTool) Description(ctx tool.Context) string {
	return "MCP 서버에서 리소스 목록을 가져옵니다."
}

func (t *ListMcpResourcesTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"server": map[string]any{"type": "string", "description": "MCP 서버 이름"},
		},
		"required": []string{"server"},
	}
}

func (t *ListMcpResourcesTool) IsReadOnly() bool { return true }

func (t *ListMcpResourcesTool) MaxResultSizeChars() int {
	return 100000
}

func (t *ListMcpResourcesTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	return tool.DefaultAllowPermission(), nil
}

func (t *ListMcpResourcesTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	var req struct {
		Server string `json:"server"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	ch := make(chan tool.ToolEvent, 2)
	go func() {
		defer close(ch)
		manager := mcp.NewManager(filepath.Join(constants.ConfigDir(), "mcp.json"))
		if err := manager.Load(); err != nil {
			ch <- tool.NewErrorEvent(err)
			return
		}
		server := strings.TrimSpace(req.Server)
		resources := manager.ListResources(server)
		ch <- tool.NewOutputEvent(fmt.Sprintf("resources for %s: %v", server, resources))
		ch <- tool.NewDoneEvent()
	}()
	return ch, nil
}
