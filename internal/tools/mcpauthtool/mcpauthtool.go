package mcpauthtool

import (
	"encoding/json"
	"fmt"

	tool "github.com/koreaf16/argus/internal/tools"
)

type McpAuthTool struct{}

func NewMcpAuthTool() *McpAuthTool { return &McpAuthTool{} }

func (t *McpAuthTool) Name() string { return "mcp_auth" }

func (t *McpAuthTool) Description(ctx tool.Context) string {
	return "MCP 서버 인증 흐름을 시작합니다."
}

func (t *McpAuthTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type":       "object",
		"properties": map[string]any{"server": map[string]any{"type": "string", "description": "인증할 서버 이름"}},
	}
}

func (t *McpAuthTool) IsReadOnly() bool { return false }

func (t *McpAuthTool) MaxResultSizeChars() int {
	return 100000
}

func (t *McpAuthTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	return tool.DefaultAskPermission(), nil
}

func (t *McpAuthTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	var req struct {
		Server string `json:"server"`
	}
	_ = json.Unmarshal(input, &req)
	ch := make(chan tool.ToolEvent, 2)
	go func() {
		defer close(ch)
		ch <- tool.NewOutputEvent(fmt.Sprintf("mcp auth request accepted for server %q; complete auth in external MCP client", req.Server))
		ch <- tool.NewDoneEvent()
	}()
	return ch, nil
}
