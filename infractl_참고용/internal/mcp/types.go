// Package mcp
// File: types.go
// Description: MCP 클라이언트 관련 타입 및 JSON-RPC 메시지 구조체 정의
// Responsibility: MCPServerConfig, JSON-RPC 2.0 메시지, MCPToolDef 등 MCP 프로토콜 타입

package mcp

import "encoding/json"

// MCPServerConfig는 config.yaml에서 읽는 MCP 서버 설정이다.
// config 패키지와 동일 구조체를 mcp 패키지에도 정의하여 순환 임포트를 방지한다.
type MCPServerConfig struct {
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args"`
	Env     map[string]string `yaml:"env"`
}

// JSONRPCRequest는 JSON-RPC 2.0 요청 메시지이다.
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// JSONRPCResponse는 JSON-RPC 2.0 응답 메시지이다.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError는 JSON-RPC 에러 객체이다.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCPToolDef는 MCP 서버의 tools/list 응답에서 반환되는 도구 정의이다.
type MCPToolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// initializeParams는 MCP initialize 요청 파라미터이다.
type initializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	ClientInfo      mcpClientInfo      `json:"clientInfo"`
	Capabilities    mcpCapabilities    `json:"capabilities"`
}

type mcpClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type mcpCapabilities struct{}

// toolsListResult는 tools/list 응답 결과이다.
type toolsListResult struct {
	Tools []MCPToolDef `json:"tools"`
}

// toolsCallParams는 tools/call 요청 파라미터이다.
type toolsCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// toolsCallResult는 tools/call 응답 결과이다.
type toolsCallResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError"`
}

// toolContent는 도구 실행 결과의 콘텐츠 항목이다.
type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Status는 MCP 클라이언트의 연결 상태를 나타낸다.
type Status string

const (
	StatusConnected    Status = "connected"
	StatusConnecting   Status = "connecting"
	StatusError        Status = "error"
	StatusDisconnected Status = "disconnected"
)
