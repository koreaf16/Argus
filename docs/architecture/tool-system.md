# Tool 시스템

## 1. Tool 인터페이스

```go
type Tool interface {
    Name() string
    Description(ctx Context) string
    InputSchema() ToolInputJSONSchema
    IsReadOnly() bool
    Call(ctx Context, input json.RawMessage) (<-chan ToolEvent, error)
    CheckPermission(ctx Context, input json.RawMessage) (PermissionResult, error)
    MaxResultSizeChars() int
}
```

## 2. ToolEvent

| Kind | 설명 |
|------|------|
| `output` | 최종 출력 |
| `chunk` | 스트리밍 청크 (display-only) |
| `error` | 에러 |
| `done` | 완료 |
| `password_prompt` | 비밀번호 요청 |
| `ask_user_prompt` | 사용자 질문 |
| `ask_user_batch_prompt` | 배치 사용자 질문 |

## 3. Registry

```go
type Registry struct {
    mu     sync.RWMutex
    tools  map[string]Tool    // 도구 맵
    source map[string]string  // 출처 추적 ("" / "mcp:xxx")
    alias  map[string]string  // 별칭 매핑
}
```

### 메서드

| 메서드 | 설명 |
|--------|------|
| `Register(t Tool)` | 도구 등록 |
| `RegisterFromMCP(server, t Tool)` | MCP 도구 등록 |
| `RegisterAlias(alias, target)` | 별칭 등록 |
| `Lookup(name)` | 도구 조회 |
| `ToolSpecs(ctx)` | LLM용 ToolSpec 배열 |
| `Unregister(name)` | 도구 제거 |
| `UnregisterServer(server)` | MCP 서버 도구 일괄 제거 |
| `ListByServer(server)` | 서버별 도구 목록 |

## 4. 내장 Tool 분류 (23개)

| 분류 | 도구 |
|------|------|
| **셸 도구** | `bash`, `powershell`, `shelljob`, `shelljobcontrol` |
| **파일/FS** | `fileread`, `filewrite`, `fslist`, `glob`, `grep` |
| **웹** | `webfetch`, `websearch` |
| **계획/관리** | `task_create`, `task_update`, `enter_plan_mode`, `exit_plan_mode` |
| **외부 통합** | `mcptool`, `lsptool`, `listmcpresources`, `readmcpresource`, `mcpauth` |
| **서버 관리** | `serverconnect`, `servercopy`, `serverinspect`, `servermetrics`, `servertunnel` |
| **기타** | `skilltool`, `sniptool`, `askuser`, `enterworktree`, `exitworktree` |

## 5. Tool Renderer (TUI 도구 전용 UI)

```go
type ToolRenderer interface {
    RenderToolUse(args, streamBody, theme) string
    RenderToolResult(resultText, durationMs, theme) string
    CreateInteractiveModel(args, theme) InteractiveModel  // nil 이면 정적만
}
```

### InteractiveModel

BubbleTea Model 확장 — 상호작용이 필요한 도구(SSH 비밀번호, ask_user 등)에 사용됩니다.

| 메서드 | 설명 |
|--------|------|
| `Init()` | 초기화 |
| `Update(msg)` | 메시지 처리 |
| `View()` | 렌더링 |
| `SetFocus/IsFocused()` | 포커스 상태 |
| `SetExpanded/IsExpanded()` | 펼침 상태 |
| `SetFinished/IsFinished()` | 완료 상태 |
| `OnStreamDelta(delta)` | 스트리밍 업데이트 |
| `SetInputResponse(chan)` | 입력 채널 설정 |

### 구현 예시

| 파일 | 도구 |
|------|------|
| `webfetch/ui.go` | WebFetch 전용 UI |
| `websearch/ui.go` | WebSearch 전용 UI |
| `specialized_renderers.go` | 기타 도구 렌더러 |
