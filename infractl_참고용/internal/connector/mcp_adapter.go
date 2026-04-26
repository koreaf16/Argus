// Package connector
// File: mcp_adapter.go
// Description: MCP(Model Context Protocol) 서버를 시스템 커넥터로 변환하는 어댑터
// Responsibility: MCP 클라이언트 수명 주기 관리, 자격증명 주입, 도구 세트 생성

package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/yourorg/infractl/internal/mcp"
	"github.com/yourorg/infractl/internal/tools"
)

// McpConnector는 외부 MCP 서버를 표준 Connector 인터페이스로 래핑한다.
type McpConnector struct {
	client         *mcp.Client
	confirmHandler ConfirmationHandler
	serviceType    string
	toolNames      []string
}

// NewMcpConnector는 새로운 MCP 어댑터를 생성한다.
func NewMcpConnector() *McpConnector {
	return &McpConnector{}
}

// SetConfirmationHandler는 사용자 승인 핸들러를 설정한다.
func (c *McpConnector) SetConfirmationHandler(h ConfirmationHandler) {
	c.confirmHandler = h
}

// ServiceType은 이 커넥터가 담당하는 서비스 타입을 반환한다.
func (c *McpConnector) ServiceType() string {
	return c.serviceType
}

// GenerateTools는 MCP 서버에 연결하고 제공되는 도구 목록을 반환한다.
func (c *McpConnector) GenerateTools(info ServiceInfo, creds Credentials) []tools.Tool {
	c.serviceType = info.ServiceType
	ctx := context.Background()

	// 1. MCP 설정 구성 (info.Details 및 creds 기반)
	config := c.buildConfig(info, creds)

	// 2. 외부 커넥터 실행 전 사용자 승인 (Confirmation)
	if c.confirmHandler != nil {
		title := fmt.Sprintf("외부 커넥터 실행 승인 (%s)", info.ServiceType)
		message := fmt.Sprintf("타겟 %s에서 다음 명령을 실행하여 외부 커넥터를 활성화하시겠습니까?\n\n명령: %s %s",
			info.ServerName, config.Command, strings.Join(config.Args, " "))
		
		approved, err := c.confirmHandler.Confirm(ctx, title, message)
		if err != nil || !approved {
			slog.Warn("mcp connector: activation cancelled by user", "server", info.ServerName, "type", info.ServiceType)
			return nil
		}
	}
	
	// 3. MCP 클라이언트 생성 및 연결
	// 이름은 "server/type/name" 형식을 사용
	clientName := fmt.Sprintf("%s/%s", info.ServerName, info.ServiceType)
	c.client = mcp.NewClient(clientName, config)

	if err := c.client.Connect(ctx); err != nil {
		slog.Error("mcp connector: failed to connect", "server", clientName, "err", err)
		return nil
	}

	// 4. 도구 목록 추출 및 캐싱
	mcpTools := c.client.ToRegistryTools()
	c.toolNames = make([]string, 0, len(mcpTools))
	for _, t := range mcpTools {
		c.toolNames = append(c.toolNames, t.Name())
	}

	return mcpTools
}

// ToolNames는 활성화된 MCP 도구 이름 목록을 반환한다.
func (c *McpConnector) ToolNames() []string {
	return c.toolNames
}

// Status는 현재 MCP 서버의 연결 상태를 반환한다.
func (c *McpConnector) Status() ConnectorStatus {
	if c.client == nil {
		return StatusDisconnected
	}
	return ConnectorStatus(c.client.Status)
}

// buildConfig는 ServiceInfo.Details와 Credentials를 조합하여 MCP 실행 설정을 만든다.
func (c *McpConnector) buildConfig(info ServiceInfo, creds Credentials) mcp.MCPServerConfig {
	// 기본값: Details에 직접 명령어 정보가 있는 경우
	command := info.Details["mcp_command"]
	var args []string
	if argsStr := info.Details["mcp_args"]; argsStr != "" {
		_ = json.Unmarshal([]byte(argsStr), &args)
	}

	// 매핑 레지스트리 (간이 구현 - 나중에 외부 파일로 분리 가능)
	// info.ServiceType이 "mcp-postgres" 형태일 때 자동 매핑
	if command == "" {
		switch info.ServiceType {
		case "mcp-postgres":
			command = "npx"
			args = []string{"-y", "@modelcontextprotocol/server-postgres"}
		case "mcp-redis":
			command = "npx"
			args = []string{"-y", "@kangw/redis-mcp-server"}
		case "mcp-memory":
			command = "npx"
			args = []string{"-y", "@modelcontextprotocol/server-memory"}
		}
	}

	// 환경변수 구성 및 자격증명 주입
	env := make(map[string]string)
	if envStr := info.Details["mcp_env"]; envStr != "" {
		_ = json.Unmarshal([]byte(envStr), &env)
	}

	// Credentials 매핑 (Details에 env_user, env_password 등의 힌트가 있는 경우)
	if envUserKey := info.Details["env_user"]; envUserKey != "" && creds.Username != "" {
		env[envUserKey] = creds.Username
	}
	if envPassKey := info.Details["env_password"]; envPassKey != "" && creds.Password != "" {
		env[envPassKey] = creds.Password
	}
	
	// 일반적인 관례에 따른 자동 매핑 (힌트가 없을 때의 폴백)
	if _, exists := env["POSTGRES_PASSWORD"]; !exists && info.ServiceType == "mcp-postgres" && creds.Password != "" {
		env["POSTGRES_PASSWORD"] = creds.Password
	}

	return mcp.MCPServerConfig{
		Command: command,
		Args:    args,
		Env:     env,
	}
}
