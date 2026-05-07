# 상태 관리 (State Management)

## 1. AppState

[`internal/state/app_state.go`](internal/state/app_state.go:24) 는 전체 애플리케이션 상태를 관리합니다.

```go
type AppState struct {
    mu          sync.RWMutex
    Messages    []Message
    Mode        string  // "normal", "plan"
    Permissions string  // "ask", "allow", "deny"
    metadata    map[string]interface{}
}
```

## 2. ModelState

[`internal/state/model_state.go`](internal/state/model_state.go) 는 활성 모델과 관련 상태를 관리합니다.

| 메서드 | 설명 |
|--------|------|
| `SetActiveModel(alias, display, contextWin)` | 활성 모델 설정 |
| `ActiveModel()` | 활성 모델 정보 반환 |
| `ActiveModelContext()` | 컨텍스트 윈도우 크기 |
| `SetContextUsedPercent(pct)` | 컨텍스트 사용률 설정 |
| `ContextUsedPercent()` | 컨텍스트 사용률 반환 |
| `SetPermissionMode(mode)` | 권한 모드 설정 |
| `InPlanMode()` | 계획 모드 여부 |
| `GetEffortLevel()` | 노력 수준 |
| `ActiveModelProvider()` | 모델 공급자 |

## 3. SessionState

세션 관련 메타데이터를 관리합니다.

| 메서드 | 설명 |
|--------|------|
| `SetSessionID(id)` | 세션 ID 설정 |
| `SessionID()` | 세션 ID 반환 |
| `SetActiveMCPServers(servers)` | 활성 MCP 서버 |
| `ActiveMCPServers()` | 활성 MCP 서버 목록 |
| `SetActiveSkills(skills)` | 활성 스킬 |
| `ActiveSkills()` | 활성 스킬 목록 |
| `SetActiveWorkspace(alias)` | 활성 워크스페이스 |
| `ActiveWorkspace()` | 활성 워크스페이스 |

## 4. PermissionState

[`internal/state/permission_state.go`](internal/state/app_state_store.go) 는 세션 권한 규칙과 작업 디렉터리를 관리합니다.

| 메서드 | 설명 |
|--------|------|
| `AddSessionPermissionRule(rule)` | 세션 권한 규칙 추가 |
| `SessionPermissionRules()` | 세션 규칙 목록 |
| `ClearSessionPermissionRules()` | 세션 규칙 삭제 |
| `SetAdditionalWorkingDirectories(dirs)` | 추가 작업 디렉터리 |
| `AdditionalWorkingDirectories()` | 추가 디렉터리 목록 |
| `SetPrePlanMode(mode)` | 계획 모드 이전 권한 저장 |
| `PrePlanMode()` | 이전 권한 모드 |
| `ClearPrePlanMode()` | 이전 권한 모드 삭제 |

## 5. 메타데이터 키

| 키 | 값 타입 | 설명 |
|----|---------|------|
| `active_model_alias` | string | 활성 모델 별칭 |
| `active_model_name` | string | 모델 표시명 |
| `active_model_context` | int | 컨텍스트 윈도우 크기 |
| `active_model_provider` | string | LLM 공급자 |
| `context_used_percent` | int | 컨텍스트 사용률 (%) |
| `effort_level` | string | 노력 수준 |
| `session_id` | string | 세션 UUID |
| `active_mcp_servers` | []string | 활성 MCP 서버 목록 |
| `active_skills` | []string | 활성 스킬 목록 |
| `active_workspace` | string | 활성 워크스페이스 |
| `session_permission_rules` | []PermissionRule | 세션 권한 규칙 |
| `additional_working_directories` | []string | 추가 작업 디렉터리 |
| `pre_plan_mode` | string | 계획 모드 이전 권한 모드 |

## 6. TodoState

[`internal/state/todo_state.go`](internal/state/todo_state.go) 는 세션별 태스크(Todo) 목록을 관리합니다. 이 데이터는 TUI 하단의 태스크 리스트 패널을 렌더링하는 데 사용됩니다.

| 메서드 | 설명 |
|--------|------|
| `SetTodos(sessionID, todos)` | 특정 세션의 Todo 목록 설정 |
| `Todos(sessionID)` | 특정 세션의 Todo 목록 반환 |
