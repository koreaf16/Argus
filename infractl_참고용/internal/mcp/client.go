// Package mcp
// File: client.go
// Description: MCP 클라이언트 — 외부 MCP 서버에 연결하여 도구를 가져오고 호출한다
// Responsibility: Connect/ListTools/CallTool/Close 및 ConnectorStatus 상태 추적

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/yourorg/infractl/internal/tools"
)

// Client는 단일 외부 MCP 서버의 연결과 도구 목록을 관리한다.
type Client struct {
	Name      string
	Status    Status
	transport *StdioTransport
	toolDefs  []MCPToolDef
}

// NewClient는 MCPServerConfig로 Client를 생성한다.
// Connect()를 호출해야 실제 연결이 수행된다.
func NewClient(name string, cfg MCPServerConfig) *Client {
	return &Client{
		Name:      name,
		Status:    StatusDisconnected,
		transport: NewStdioTransport(cfg.Command, cfg.Args, cfg.Env),
	}
}

// Connect는 MCP 서버 서브프로세스를 기동하고 initialize 핸드셰이크를 수행한다.
func (c *Client) Connect(ctx context.Context) error {
	c.Status = StatusConnecting

	if err := c.transport.Start(); err != nil {
		c.Status = StatusError
		return fmt.Errorf("start mcp transport: %w", err)
	}

	params := initializeParams{
		ProtocolVersion: "2024-11-05",
		ClientInfo:      mcpClientInfo{Name: "infractl", Version: "1.0.0"},
		Capabilities:    mcpCapabilities{},
	}

	if _, err := c.transport.Send(ctx, "initialize", params); err != nil {
		c.Status = StatusError
		return fmt.Errorf("mcp initialize: %w", err)
	}

	// initialized 알림 전송 (선택적이지만 일부 서버에서 필요)
	_ = c.transport.Notify("notifications/initialized", nil)

	toolDefs, err := c.ListTools(ctx)
	if err != nil {
		c.Status = StatusError
		return fmt.Errorf("list mcp tools: %w", err)
	}

	c.toolDefs = toolDefs
	c.Status = StatusConnected
	slog.Info("mcp client connected", "server", c.Name, "tools", len(c.toolDefs))
	return nil
}

// ListTools는 MCP 서버의 도구 목록을 조회한다.
func (c *Client) ListTools(ctx context.Context) ([]MCPToolDef, error) {
	raw, err := c.transport.Send(ctx, "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("tools/list: %w", err)
	}

	var result toolsListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse tools/list response: %w", err)
	}
	return result.Tools, nil
}

// CallTool은 MCP 서버의 도구를 호출하고 결과 텍스트를 반환한다.
func (c *Client) CallTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	if c.Status != StatusConnected {
		// 연결이 끊겼으면 재연결 시도
		if err := c.Connect(ctx); err != nil {
			return fmt.Sprintf("MCP 서버 재연결 실패: %s", err), nil
		}
	}

	params := toolsCallParams{
		Name:      toolName,
		Arguments: args,
	}

	raw, err := c.transport.Send(ctx, "tools/call", params)
	if err != nil {
		c.Status = StatusError
		return fmt.Sprintf("tools/call 오류: %s", err), nil
	}

	var result toolsCallResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Sprintf("응답 파싱 오류: %s", err), nil
	}

	if result.IsError {
		parts := make([]string, 0, len(result.Content))
		for _, c := range result.Content {
			parts = append(parts, c.Text)
		}
		return fmt.Sprintf("[MCP 오류] %s", strings.Join(parts, "\n")), nil
	}

	parts := make([]string, 0, len(result.Content))
	for _, c := range result.Content {
		if c.Type == "text" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n"), nil
}

// ToRegistryTools는 이 MCP 클라이언트의 도구를 tools.Tool 슬라이스로 변환한다.
// 각 도구는 mcp__{serverName}__{toolName} 형식으로 등록된다.
func (c *Client) ToRegistryTools() []tools.Tool {
	result := make([]tools.Tool, 0, len(c.toolDefs))
	for _, def := range c.toolDefs {
		def := def // 클로저 캡처
		result = append(result, newMCPTool(c.Name, def, c))
	}
	return result
}

// Reconnect는 기존 연결을 닫고 재연결을 수행한다.
func (c *Client) Reconnect(ctx context.Context) error {
	_ = c.transport.Close()
	c.Status = StatusDisconnected
	return c.Connect(ctx)
}

// Close는 MCP 서버 연결을 종료한다.
func (c *Client) Close() error {
	_ = c.transport.Notify("notifications/exit", nil)
	c.Status = StatusDisconnected
	return c.transport.Close()
}
