# internal/tools — 도구 시스템

## 파일 구조

| 파일 | 역할 |
|------|------|
| `tool.go` | `Tool` 인터페이스, `ToolEvent`, `PermissionResult` |
| `registry.go` | `Registry`: 도구 등록/조회/별칭/MCP 소스 추적 |
| `context.go` | `Context`: Tool 실행 시 컨텍스트 |
| `path_guard.go` | 경로 보호 공통 로직 |
| `path_resolvers.go` | 경로 해결 헬퍼 |
| `permission_policy.go` | 공통 권한 정책 |
| `workspace_support.go` | 워크스페이스 지원 |

## Tool 인터페이스

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

## 내장 도구 목록

### 셸 도구 (`tools/bash/`, `tools/powershell/`)

| 도구 | 패키지 | 설명 |
|------|--------|------|
| `bash` | `tools/bash` | Bash 명령 실행 (Tree-sitter 파서, 30+ 파일) |
| `powershell` | `tools/powershell` | PowerShell 명령 실행 |
| `shelljob` | `tools/shelljob` | 백그라운드 작업 시작 |
| `shelljobcontrol` | `tools/shelljobcontrol` | 백그라운드 작업 제어 |

### 파일 도구

| 도구 | 설명 |
|------|------|
| `fileread` | 파일 읽기 (라인 범위, root 경계 강제) |
| `filewrite` | 파일 쓰기 (atomic write, root 경계 강제) |
| `fslist` | 디렉토리 목록 |
| `glob` | 파일 패턴 검색 (root 경계 강제) |
| `grep` | 텍스트 검색 (`rg` 우선, Go 폴백) |

### 웹 도구 (`tools/webfetch/`, `tools/websearch/`)

| 도구 | 설명 |
|------|------|
| `webfetch` | 웹 페이지 페칭 (SSRF 보호) |
| `websearch` | 웹 검색 (DDG Lite + SearXNG + Provider intent) |

### 계획/관리 도구

| 도구 | 설명 |
|------|------|
| `task_create` | 태스크(Task) 생성 및 작성 |
| `task_update` | 태스크(Task) 상태 갱신 |
| `enter_plan_mode` | 계획 모드(Plan Mode) 진입 |
| `exit_plan_mode` | 계획 모드(Plan Mode) 종료 |

### 외부 통합

| 도구 | 설명 |
|------|------|
| `mcptool` | MCP 도구 실행 |
| `lsptool` | LSP (Hover/Diagnostics) |
| `listmcpresources` | MCP 리소스 목록 |
| `readmcpresource` | MCP 리소스 읽기 |
| `mcpauth` | MCP 인증 |

### 서버 관리

| 도구 | 설명 |
|------|------|
| `serverconnect` | 원격 서버 연결 |
| `servercopy` | 서버 간 파일 복사 |
| `serverinspect` | 서버 정보 조회 |
| `servermetrics` | 서버 메트릭스 |
| `servertunnel` | SSH 터널 |

### 기타

| 도구 | 설명 |
|------|------|
| `skilltool` | Skill 실행 |
| `sniptool` | 컨텍스트 단축 |
| `askuser` | 사용자 질문 |
| `enterworktree` | 작업 트리 진입 |
| `exitworktree` | 작업 트리退出 |
