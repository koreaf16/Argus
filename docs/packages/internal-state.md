# internal/state — 상태 관리

## 파일 구조

| 파일 | 역할 |
|------|------|
| `app_state.go` | `AppState`: 전체 애플리케이션 상태 |
| `app_state_store.go` | AppState 저장소 (metadata, permission, session) |
| `model_state.go` | `ModelState`: 활성 모델/권한/컨텍스트 상태 |
| `todo_state.go` | `TodoState`: 세션 Todo 목록 |

## AppState

```go
type AppState struct {
    mu          sync.RWMutex
    Messages    []Message
    Mode        string  // "normal", "plan"
    Permissions string  // "ask", "allow", "deny"
    metadata    map[string]interface{}
}
```

## ModelState

| 메서드 | 설명 |
|--------|------|
| `SetActiveModel(alias, display, contextWin)` | 활성 모델 설정 |
| `ActiveModel()` | 활성 모델 정보 |
| `ActiveModelContext()` | 컨텍스트 윈도우 크기 |
| `SetContextUsedPercent(pct)` | 컨텍스트 사용률 설정 |
| `ContextUsedPercent()` | 컨텍스트 사용률 |
| `SetPermissionMode(mode)` | 권한 모드 |
| `InPlanMode()` | 계획 모드 여부 |
| `GetEffortLevel()` | 노력 수준 |
| `ActiveModelProvider()` | 모델 공급자 |

## SessionState

| 메서드 | 설명 |
|--------|------|
| `SetSessionID(id)` | 세션 ID |
| `SessionID()` | 세션 ID 반환 |
| `SetActiveMCPServers(servers)` | 활성 MCP 서버 |
| `ActiveMCPServers()` | MCP 서버 목록 |
| `SetActiveWorkspace(alias)` | 활성 워크스페이스 |
| `ActiveWorkspace()` | 워크스페이스 |

## PermissionState

| 메서드 | 설명 |
|--------|------|
| `AddSessionPermissionRule(rule)` | 세션 규칙 추가 |
| `SessionPermissionRules()` | 세션 규칙 목록 |
| `ClearSessionPermissionRules()` | 세션 규칙 삭제 |
| `SetAdditionalWorkingDirectories(dirs)` | 추가 디렉터리 |
| `AdditionalWorkingDirectories()` | 추가 디렉터리 목록 |
| `SetPrePlanMode(mode)` | 계획 모드 이전 저장 |
| `PrePlanMode()` | 이전 모드 |

## TodoState

| 메서드 | 설명 |
|--------|------|
| `SetTodos(todos)` | Todo 목록 설정 |
| `GetTodos()` | Todo 목록 반환 |
| `ClearTodos()` | Todo 목록 삭제 |
