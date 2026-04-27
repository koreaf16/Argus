# internal/query — 쿼리 엔진

## 파일 구조

| 파일 | 역할 |
|------|------|
| `engine.go` | `Engine` 구조체, `SubmitMessage`, `run()` 루프, Tool 호출 |
| `config.go` | `Config` 구조체 (MaxTokens, MaxToolIterations, DebugTools) |
| `deps.go` | `Deps` 구조체 (외부 종속성 주입) |
| `context_helpers.go` | 시스템 프롬프트 조립 헬퍼 |

## Engine 구조체

```go
type Engine struct {
    llm          llm.LLM             // LLM 클라이언트
    registry     *tool.Registry      // Tool 레지스트리
    hookRegistry *tools.HookRegistry // 훅 레지스트리
    hookDispatcher *hooks.HookDispatcher // 훅 디스패처
    fileWatcher  *hooks.FileWatcher  // 파일 감시
    state        *state.AppState     // 앱 상태
    messages     []llm.Message       // legacy 메시지
    systemFn     func() []llm.SystemBlock
    cfg          Config
    deps         Deps
    budget       *TokenBudget
    // Context Graph
    graph      *ctxpkg.Graph
    est        *ctxpkg.TokenEstimator
    distiller  *ctxpkg.Distiller
    artStore   *ctxpkg.ArtifactStore
    artMF      *ctxpkg.ArtifactManifest
    // Permission 캐시
    permissionRulesCache    []types.PermissionRule
    permissionRulesCachedAt time.Time
}
```

## 주요 API

| 메서드 | 설명 |
|--------|------|
| `NewEngine(client, registry, appState, systemFn)` | 엔진 생성 |
| `SetLLM(client)` | LLM 클라이언트 교체 |
| `SetConfig(cfg)` | 설정 교체 |
| `SetDeps(ctx, deps)` | 종속성 주입 |
| `SubmitMessage(ctx, input)` | 사용자 메시지 제출 (비동기) |

## Config

| 필드 | 기본값 | 설명 |
|------|--------|------|
| `MaxTokens` | 2048 | 최대 출력 토큰 |
| `MaxToolIterations` | 100 | 턴 당 최대 Tool 반복 |
| `DebugTools` | false | Tool 목록 디버그 출력 |
| `ContextFallback` | 200K | 컨텍스트 초과 시 폴백 |

## Deps

| 필드 | 설명 |
|------|------|
| `ApproveTool` | Tool 권한 승인 콜백 |
| `WorkingDir` | 작업 디렉터리 |
| `Workspace` | 워크스페이스 참조 |
| `ShellJobs` | 백그라운드 작업 매니저 |
| `AIDebug` | NDJSON 트레이싱 활성화 |
| `BaseDir` | 설정 디렉토리 |
| `SessionID` | 세션 ID |

## System Prompt

`DefaultSystemPrompt()`와 `systemFn()` 은 LLM 시스템 메시지를 생성합니다:
- Argus 에이전트 지침
- Workspace 시스템 블록
- 활성 도구 목록
