# 쿼리 엔진 (Query Engine)

[`internal/query/engine.go`](internal/query/engine.go) 에 정의된 `Engine` 구조체는 Argus의 **핵심 심장**으로, 모든 LLM 상호작용과 Tool 실행을 조율합니다.

## 1. Engine 구조체

```go
type Engine struct {
    mu sync.RWMutex

    // LLM
    llm llm.LLM

    // Tool
    registry *tool.Registry
    hookRegistry *tools.HookRegistry

    // Hook
    hookDispatcher *hooks.HookDispatcher
    fileWatcher *hooks.FileWatcher

    // State
    state *state.AppState
    messages []llm.Message  // legacy: snapshot load/save 호환
    systemFn func() []llm.SystemBlock

    cfg  Config
    deps Deps
    budget *TokenBudget

    // Context management (Graph-based)
    graph *ctxpkg.Graph
    est   *ctxpkg.TokenEstimator
    distiller *ctxpkg.Distiller
    artStore  *ctxpkg.ArtifactStore
    artMF     *ctxpkg.ArtifactManifest

    // Debug / Permission cache
    permissionRulesCache []types.PermissionRule
    permissionRulesCachedAt time.Time
}
```

### Config (`config.go`)
| 필드 | 기본값 | 설명 |
|------|--------|------|
| `MaxTokens` | 2048 | 최대 출력 토큰 |
| `MaxToolIterations` | 100 | 턴 당 최대 Tool 반복 |
| `DebugTools` | false | Tool 목록 디버그 출력 |
| `ContextFallback` | 200K | 컨텍스트 초과 시 폴백 |

### Deps (`deps.go`)
| 필드 | 설명 |
|------|------|
| `ApproveTool` | Tool 권한 승인 콜백 |
| `WorkingDir` | 작업 디렉터리 |
| `Workspace` | 워크스페이스 참조 |
| `ShellJobs` | 백그라운드 작업 매니저 |
| `AIDebug` | NDJSON 트레이싱 활성화 |

## 2. Run Loop (`SubmitMessage` → `run()`)

```
SubmitMessage(ctx, input)
  │
  ├── HookDispatch(UserPrompt)  ← 차단 시 UIEventError 반환
  │
  ├── graph.AppendUser(input)   ← User 노드 추가
  │
  └── go e.run()  ← 별도 goroutine에서 실행
        │
        └── for iter < MaxToolIterations (100)
              │
              ├── 1. RenderForLLM()
              │     ├── graph.MarkProtected(currentUserText)
              │     ├── systemFn() + workspaceSystemBlocks()
              │     └── graph nodes → []llm.Message (budget 기반 선택)
              │
              ├── 2. ToolSpecs 가져오기
              │     └── Plan Mode면 Read-Only 도구만 필터
              │
              ├── 3. ToolChoice 결정
              │     ├── iter==0: classifierLLM으로 의도 분류
              │     └── iter>0: 항상 "auto"
              │
              ├── 4. LLM Request 빌드
              │
              ├── 5. 토큰 수 계산
              │     ├── ContextWindow 초과면 ForceConsolidate()
              │     └── 재Render 후 재시도
              │
              ├── 6. client.Stream() 호출
              │     ├── EventThinkingDelta  → UIEvent
              │     ├── EventTextDelta      → UIEvent
              │     ├── EventToolUseStart   → toolCalls[]
              │     ├── EventStop           → stopReason
              │     └── EventError          → UIEventError
              │
              ├── 7. toolCalls == 0 (종료)
              │     ├── 첫 반복 도구 없이 텍스트만 → 강제 재시도
              │     └── 아니라면 → UIEventDone 반환
              │
              └── 8. toolCalls > 0 (도구 실행)
                    ├── RunToolsWithDispatcher (병렬 실행)
                    ├── Distiller.Distill (계층적 압축)
                    ├── graph.AppendToolResult
                    └── 다음 반복으로 계속
```

## 3. Tool 호출 파이프라인 (`invokeTool`)

```
invokeTool()
  │
  ├── Tool Lookup  ← Registry.Lookup(name)
  │
  ├── Plan Mode 체크  ← 차단 시 "tool blocked in plan mode"
  │
  ├── Pre-Approval Rule 체크
  │     ├── Disk 규칙 (~/.argus/permissions/) — 2초 TTL 캐시
  │     ├── Session 규칙 (AppState)
  │     └── 미매칭 → Tool.CheckPermission()
  │           ├── Allow → 직접 실행
  │           ├── Deny → "permission denied"
  │           └── Passthrough
  │                 ├── ReadOnly? → Allow
  │                 └── BehaviorAsk → ApprovalGate.Prompt
  │                       ├── Allow → 실행
  │                       └── Deny → "permission denied"
  │
  ├── toolImpl.Call()
  │
  ├── ToolEvent 스트림 처리
  │     ├── output  → 최종 출력
  │     ├── chunk   → 스트리밍 청크 (display-only)
  │     ├── error   → 에러
  │     ├── done    → 완료
  │     ├── password_prompt → 비밀번호 요청
  │     ├── ask_user_prompt → 사용자 질문
  │     └── ask_user_batch_prompt → 배치 질문
  │
  └── Distill()  ← 계층적 압축
        ├── Full (<=8,000 chars)  → inline 그대로
        ├── Partial (8~20K)       → head 80줄 + tail 60줄
        └── Summary (>20K)        → LLM 요약 시도
              ├── 성공 → Summary 노드
              └── 실패 → Extractive (head 5 + tail 5)
```

## 4. Stop Hooks (턴 종료 조건)

[`stop_hooks.go`](internal/query/stop_hooks.go) 는 턴 종료 시점을 제어합니다.

- **첫 반복에서 tool 없이 텍스트만 반환**: 강제 재시도
- **web evidence forced continuation**: 검색 후 계속
- **StopReasonStopped / StopReasonEndTurn**: 정상 종료
- **MaxToolIterations 초과**: 최대 반복 도달

## 5. Plan Mode 동작

- Plan 모드 진입 (`EnterPlanMode`): 이전 권한 모드 저장 → 계획 모드 설정 → 세션 plan 파일 초기화
- Plan 모드 실행: Read-Only 도구만 허용 (`TaskCreate`, `ExitPlanMode` 제외)
- Plan 모드退出 (`ExitPlanMode`): `plan_execution_ready` 이벤트 발화
- REPL은 승인된 단계들을 순차적으로 실행 (단계별 확인)
- 실행 중 Todo 상태 동기화 (`pending` → `in_progress` → `completed`)
