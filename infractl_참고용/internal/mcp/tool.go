// Package mcp
// File: tool.go
// Description: MCP 도구 래퍼 — tools.Tool 인터페이스를 구현하는 MCP 도구 어댑터
// Responsibility: MCPToolDef를 tools.Tool로 래핑하여 에이전트 레지스트리에 등록 가능하게 함

package mcp

import (
	"context"
	"fmt"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/tools"
)

// mcpTool은 MCP 서버 도구 하나를 tools.Tool 인터페이스로 래핑한다.
type mcpTool struct {
	serverName string
	def        MCPToolDef
	client     *Client
}

// newMCPTool은 MCP 도구 래퍼를 생성한다.
func newMCPTool(serverName string, def MCPToolDef, client *Client) *mcpTool {
	return &mcpTool{
		serverName: serverName,
		def:        def,
		client:     client,
	}
}

// Name은 `mcp__{serverName}__{toolName}` 형식의 도구 이름을 반환한다.
func (t *mcpTool) Name() string {
	return fmt.Sprintf("mcp__%s__%s", t.serverName, t.def.Name)
}

func (t *mcpTool) Description() string {
	return fmt.Sprintf("[MCP:%s] %s", t.serverName, t.def.Description)
}

func (t *mcpTool) Parameters() map[string]interface{} {
	if t.def.InputSchema != nil {
		return t.def.InputSchema
	}
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}

func (t *mcpTool) IsReadOnly() bool { return true }

// IsEnabled는 MCP 클라이언트가 connected 상태일 때만 true를 반환한다.
func (t *mcpTool) IsEnabled() bool {
	return t.client.Status == StatusConnected
}

// Execute는 MCP 서버에 tools/call을 전송하고 결과를 반환한다.
// executor.Executor 파라미터는 MCP 도구에서 사용하지 않는다.
func (t *mcpTool) Execute(ctx context.Context, args map[string]interface{}, _ executor.Executor) (tools.ToolOutcome, error) {
	// "target" 키는 MCP 서버에 전달하지 않는다
	cleanArgs := make(map[string]interface{}, len(args))
	for k, v := range args {
		if k != "target" {
			cleanArgs[k] = v
		}
	}
	result, err := t.client.CallTool(ctx, t.def.Name, cleanArgs)
	if err != nil {
		return tools.ToolOutcome{}, err
	}
	return tools.ToolOutcome{Content: result, Success: true}, nil
}
