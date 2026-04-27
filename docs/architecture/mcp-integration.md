# MCP (Model Context Protocol) 통합

## 1. MCP 설정

`~/.argus/mcp.json` 형식:

```json
{
  "servers": [
    {
      "name": "filestash",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-file-stash"],
      "env": {"HOME": "/home/user"},
      "tools": [
        {"name": "read_file", "description": "Read a file"},
        {"name": "write_file", "description": "Write a file"}
      ],
      "resources": [
        {"uri": "file:///home/user/docs", "name": "Documents"}
      ]
    }
  ]
}
```

## 2. MCP Manager

[`internal/services/mcp/manager.go`](internal/services/mcp/manager.go:40) 는 MCP 설정을 로드하고 관리합니다.

```go
type Manager struct {
    path    string                // mcp.json 경로
    servers map[string]ServerConfig
}
```

### 메서드

| 메서드 | 설명 |
|--------|------|
| `Load()` | mcp.json 로드 |
| `ServerNames()` | 활성 서버 목록 |
| `ToolConfigs(server)` | 서버의 도구 설정 |
| `ResourceConfigs(server)` | 서버의 리소스 설정 |

## 3. BridgeTool 패턴

MCP 서버의 도구는 `BridgeTool`을 통해 Argus Tool 시스템에 브리지됩니다.

```
MCP 설정 파일 (mcp.json)
  │
  ▼
mcp.Manager.Load()
  │
  ├── ServerNames()    → 활성 서버 목록
  └── ToolConfigs(server) → 서버의 도구 설정
          │
          ▼
  registerMCPBridgeTools()
          │
  ┌───────┴───────┐
  ▼               ▼
기존 mcp__ 도구  BridgeTool 생성
확인            NewBridgeTool(server, name, ...)
  │               │
  └───────┬───────┘
          │
          ▼
  Registry.RegisterFromMCP()
          │
          ▼
  도구명 규칙: "mcp__{server}__{tool_name}"
  예: mcp__filestash__read_file
```

### BridgeTool 구조

```go
type BridgeTool struct {
    server  string    // MCP 서버 이름
    name    string    // 도구 이름
    desc    string    // 설명
    manager *Manager  // MCP 매니저 참조
}
```

| 속성 | 값 |
|------|-----|
| `Name()` | `"mcp__{server}__{name}"` |
| `IsReadOnly()` | `false` |
| `MaxResultSizeChars()` | 100,000 |
| `CheckPermission()` | `DefaultAskPermission` |
| `Call()` | `manager.Execute(ctx, server, name, args)` |

## 4. 런타임 리로드

`/mcp reload` 명령:

```
mcpReload()
  │
  ├── mcpManager.Load()
  ├── appState.SetActiveMCPServers()
  └── registerMCPBridgeTools()
        ├── 기존 브리지 도구 제거 (UnregisterServer)
        └── 새 브리지 도구 등록
```

## 5. MCP Slash Commands

| 명령 | 설명 |
|------|------|
| `/mcp list` | 활성 서버 목록 |
| `/mcp tools <server>` | 서버의 도구 목록 |
| `/mcp resources <server>` | 서버의 리소스 목록 |
| `/mcp read <uri>` | 리소스 내용 읽기 |
| `/mcp reload` | MCP 설정 리로드 |
