# internal/hooks — 훅 시스템

## 파일 구조

| 파일 | 역할 |
|------|------|
| `dispatcher.go` | `HookDispatcher`: 이벤트 → 훅 매칭/실행/집계 |
| `executor.go` | `Executor`: command/http/prompt 타입별 실행기 |
| `matcher.go` | `HookMatcher`: 이벤트-훅 매칭 규칙 |
| `watcher.go` | `FileWatcher`: 파일 변경 감시 |
| `config.go` | `HooksConfig`: 설정 스키마 |

## Dispatcher

```go
type HookDispatcher struct {
    config       HooksConfig    // settings.json 기반 훅
    sessionHooks HooksConfig    // 런타임 등록 훅
    sessionID    string
    workDir      string
    executor     *Executor
    onceSeen     sync.Map       // once 추적
}
```

| 메서드 | 설명 |
|--------|------|
| `NewDispatcher(cfg, sessionID, workDir)` | 디스패처 생성 |
| `SetSubQueryFn(fn)` | prompt 훅용 LLM 쿼리 함수 주입 |
| `AddSessionHook(event, matcher)` | 런타임 훅 등록 |
| `RemoveSessionHook(event, cmd)` | 런타임 훅 제거 |
| `ClearSessionHooks()` | 전체 런타임 훅 삭제 |
| `NotifyCwdChanged(ctx, newCwd)` | CwdChanged 훅 발화 |
| `Dispatch(ctx, event, input)` | 훅 실행 및 결과 집계 |

## Executor

| 타입 | 실행기 | 설명 |
|------|--------|------|
| `command` | `executeCommand()` | 셸 명령 실행 |
| `http` | `executeHTTP()` | HTTP POST 발송 |
| `prompt` | `executePrompt()` | LLM 서브 쿼리 |

## AggregatedResult

| 필드 | 타입 | 설명 |
|------|------|------|
| `Continue` | bool | true면 계속, false면 차단 |
| `BlockReason` | string | 차단 이유 |
| `Stdout` | string | LLM 컨텍스트용 출력 |
| `Warnings` | []string | 경고 목록 |

## 이벤트

| 이벤트 | 설명 |
|--------|------|
| `SessionStart` | 세션 시작 |
| `UserPrompt` | 프롬프트 제출 |
| `CwdChanged` | 작업 디렉터리 변경 |
| `FileChanged` | 파일 변경 |
| `Stop` | Turn 종료 |
