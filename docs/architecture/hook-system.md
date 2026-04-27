# Hook 시스템

## 1. Hook Dispatcher

[`internal/hooks/dispatcher.go`](internal/hooks/dispatcher.go:13) 는 이벤트를 수신해 매칭되는 훅을 실행하고 결과를 집계합니다.

```go
type HookDispatcher struct {
    mu           sync.RWMutex
    config       HooksConfig    // settings.json 기반 훅
    sessionHooks HooksConfig    // 런타임 등록 훅
    sessionID    string
    workDir      string
    executor     *Executor
    onceSeen     sync.Map       // once 추적
}
```

### 이벤트

| 이벤트 | 설명 |
|--------|------|
| `SessionStart` | 세션 시작 |
| `UserPrompt` | 사용자 프롬프트 제출 |
| `CwdChanged` | 작업 디렉터리 변경 |
| `FileChanged` | 파일 변경 |
| `Stop` | Turn 종료 |

### Dispatch 흐름

```
Dispatch(ctx, event, input)
  │
  ├── resolveCommands(config + sessionHooks, event)
  │
  ├── if 조건 필터
  │
  ├── once 체크 (세션당 1회)
  │
  ├── async?  ── Y ──▶ goroutine 실행 (비차단)
  │              │
  │              N
  │              │
  │              ▼
  │         Executor.Run()
  │              │
  │              ▼
  │         AggregatedResult 집계
  │              ├── Continue: true/false
  │              ├── BlockReason: 차단 이유
  │              ├── Stdout: LLM 컨텍스트용 출력 누적
  │              └── Warnings: 경고 누적
  │
  └── 첫 Continue=false 발견 시 즉시 중단
```

## 2. Hook Command 타입

| 타입 | 설명 | 필수 필드 | 실행기 |
|------|------|-----------|--------|
| `command` | 셸 명령 실행 | `command` | Shell 실행 |
| `http` | HTTP POST 발송 | `url` | HTTP 클라이언트 |
| `prompt` | LLM 서브 쿼리 | `prompt` | ExecuteSubQuery |

## 3. Executor

[`internal/hooks/executor.go`](internal/hooks/executor.go) 는 각 훅 타입에 맞는 실행기를 제공합니다.

| 실행기 | 설명 |
|--------|------|
| `executeCommand()` | 셸 명령 실행 (timeout, output capture) |
| `executeHTTP()` | HTTP POST 발송 (JSON body) |
| `executePrompt()` | LLM 서브 쿼리 (Engine.clone + single turn) |

## 4. Matcher

[`internal/hooks/matcher.go`](internal/hooks/matcher.go) 는 이벤트-훅 매칭 규칙을 처리합니다.

- `HookMatcher` 는 `Event` + `[]HookCommand` 쌍
- 조건: `if` 필드에서 셸 명령 평가 (true/false)

## 5. File Watcher

[`internal/hooks/watcher.go`](internal/hooks/watcher.go) 는 파일 변경을 감시합니다.

- 디렉토리/파일 레벨 감시
- `FileChanged` 훅 트리거

## 6. 설정 (HooksConfig)

`~/.argus/settings.json` 의 `hooks` 필드:

```json
{
  "hooks": {
    "UserPrompt": [
      {
        "if": "true",
        "commands": [
          {
            "type": "command",
            "command": "echo 'User prompt received'",
            "async": false
          }
        ]
      }
    ],
    "SessionStart": [
      {
        "if": "true",
        "once": true,
        "commands": [
          {
            "type": "http",
            "url": "https://example.com/webhook",
            "async": true
          }
        ]
      }
    ]
  }
}
```
