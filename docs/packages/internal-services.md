# internal/services — 외부 서비스 연동

## llm/ — LLM 공급자

| 파일 | 역할 |
|------|------|
| `llm.go` | `LLM` 인터페이스, `Request`, `Event`, `Caps` |
| `anthropic.go` | Anthropic Claude 구현 |
| `gemini.go` | Google Gemini 구현 |
| `openai.go` | OpenAI-compat 구현 |
| `registry.go` | 모델 카탈로그 (`~/.argus/models.json`) |
| `registry_catalog.go` | 내장 프리셋 |
| `registry_discovery.go` | 서버 탐색 |
| `registry_openai_probe.go` | OpenAI-compat URL probe |
| `registry_storage.go` | 카탈로그 저장/로드 |
| `toolcalls.go` | Tool call 포맷 변환 (공급자 ↔ 내장 표준) |
| `retry.go` | HTTP 재시도 (exponential backoff) |

## mcp/ — Model Context Protocol

| 파일 | 역할 |
|------|------|
| `manager.go` | MCP 설정 로드/관리 (`~/.argus/mcp.json`) |
| `bridge_tool.go` | `BridgeTool`: MCP 도구를 Argus Tool로 브리지 |
| `client.go` | MCP stdio JSON-RPC 클라이언트 |

## lsp/ — Language Server Protocol

| 파일 | 역할 |
|------|------|
| `manager.go` | LSP 프로세스 관리 (start/stop/status) |
| `client.go` | stdio JSON-RPC 클라이언트 |
| `protocol.go` | LSP 프로토콜 정의 |

## workspace/ — 원격 워크스페이스

| 파일 | 역할 |
|------|------|
| `manager.go` | 워크스페이스 관리 (연결/목록/상태) |
| `ssh_session.go` | SSH 세션 관리 |
| `ssh_probe.go` | SSH 연결 probe |
| `platform_probe.go` | 플랫폼 probe |
| `endpoint.go` | 엔드포인트 관리 |
| `credentials.go` | 인증 정보 관리 |
| `types.go` | 워크스페이스 타입 |

## tools/ — Tool 오케스트레이션

| 파일 | 역할 |
|------|------|
| `orchestration.go` | 병렬 Tool 실행 오케스트레이션 |
| `hooks.go` | Tool 훅 레지스트리 |
