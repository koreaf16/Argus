# Plan Mode + TodoWrite Enforcer 설계 (Phase G)

## 1. Plan Mode 개요

Plan Mode 는 LLM 이 mutation 도구를 실행하지 않고 탐색·계획 단계에 머물도록 강제하는 실행 모드다.

### 목적

- 복잡한 인프라 변경 작업 전 LLM 의 계획 수립 단계 분리
- 사용자 승인 없이 파괴적 명령(rm, systemctl stop, DROP TABLE 등)이 실행되지 않도록 보장
- 계획 수립 중 발생한 mutation 시도를 큐잉하여 나중에 일괄 검토·승인

---

## 2. 컴포넌트 구조

```
internal/agent/planmode/
  mode.go             ← Mode enum (Off/Active), State (sync.RWMutex + mode + queue)
  pending_queue.go    ← PendingCommand{ID, Tool, Args, At}, Queue.Add/Drain/Peek/Len/Clear
  transition.go       ← State.Enter() / Exit() / Toggle()
  readonly_filter.go  ← Allow(toolName, HookMetadata) bool — FAIL-CLOSED deny
  approval.go         ← Trigger(ctx, queue, registry, handler) ApprovalResult
```

### 데이터 흐름

```
사용자 입력
    │
    ▼
Agent.Run() ──► loop.go ──► runWithEngine()
                                │
                                ▼
                          ToolInvoker.Invoke()
                          ┌─────────────────────────────────────────────┐
                          │  1. Plan Mode 활성? → ReadOnlyFilter.Allow() │
                          │     allow → 통과                             │
                          │     deny  → Queue.Add() + deny 메시지 반환   │
                          │                                              │
                          │  2. TodoEnforcer.Enforce()                   │
                          │     todo 비었고 mutation? → deny             │
                          │                                              │
                          │  3. PreToolUse hook                          │
                          │  4. 도구 실행                                 │
                          │  5. PostToolUse hook                         │
                          └─────────────────────────────────────────────┘
```

---

## 3. ReadOnlyFilter 판정 우선순위

| 우선순위 | 조건 | 판정 |
|---------|------|------|
| 1 | `HookMetadata.ReadOnly == true` | allow |
| 2 | `HookMetadata.DiskModifying == true` | deny |
| 3 | `registry.Get(tool).IsReadOnly() == true` | allow |
| 4 | 도구 존재하지 않거나 판정 불가 | deny (FAIL-CLOSED) |

---

## 4. Pending Queue

```go
type PendingCommand struct {
    ID   string         // UUID v4
    Tool string
    Args map[string]any
    At   time.Time
}
```

- `Queue.Add(tool, args)` → UUID 생성 후 적재, `slog.Info` 기록
- `Queue.Drain()` → 전체 꺼내기 (clear 포함)
- `Queue.Peek()` → 읽기 전용 스냅샷
- `Queue.Len()` → 현재 대기 수
- 동시성: `sync.Mutex` 보호

---

## 5. Plan Mode 활성화/비활성화

### TUI (Shift+Tab)

```
Shift+Tab 입력
    │
    ├─ planMode == false → Enter() → planMode = true
    │
    └─ planMode == true
         │
         ├─ queue.Len() == 0 → Exit() → planMode = false
         │
         └─ queue.Len() > 0 → SystemMsg (pending 목록 표시)
                               → Exit() + planMode = false
                               → 사용자는 CLI로 승인/거부 처리
```

### CLI

```bash
infractl plan enter    # Plan Mode 진입 (plan_state.json 기록)
infractl plan exit     # 즉시 종료 (pending 전부 거부)
infractl plan exit --approve <id,...>   # 지정 ID 승인 후 종료
infractl plan exit --reject-all         # 전부 거부 후 종료
infractl plan status                    # 현재 상태 + pending 목록
```

---

## 6. Approval 흐름

```go
type ApprovalHandler interface {
    ShowApproval(ctx context.Context, pending []PendingCommand) (approvedIDs []string, cancelled bool)
}

type ApprovalResult struct {
    Approved []ExecutedCommand
    Rejected []PendingCommand
}
```

승인된 명령은 `registry.Get(tool).Execute(ctx, args)` 직접 호출 (LLM 재호출 없음).  
실행 결과는 세션 메시지로 append → 다음 LLM turn 이 참조.

---

## 7. TodoWrite Enforcer

### 목적

LLM 이 다단계 작업(설치/배포/마이그레이션)에서 TodoWrite 없이 곧바로 mutation 도구를 실행하는 것을 방지한다.

### 신호어 목록 (`internal/agent/todo/signals.go`)

| 한국어 | 영어 |
|--------|------|
| 설치, 배포, 마이그레이션, 업그레이드 | install, deploy, migrate, upgrade |
| 롤백, 구성, 설정, 프로비저닝 | rollback, configure, provision |
| 이관, 이전 | (한국어만) |

### Prompt Injector

신호어 감지 시 system prompt 에 아래 한 줄 추가:

```
[INSTRUCTION] This is a multi-step task. Use the todo_write tool FIRST to create a plan before executing any commands.
```

### Enforcer 판정 로직

```
Enforce(toolName, isReadOnly):
    isReadOnly == true  → allow (읽기 도구는 항상 허용)
    store.List() 비어있지 않음 → allow
    store.List() 비어있음 → deny ("TodoWrite 먼저")
```

### ToolInvoker 실행 순서

```
1. Plan Mode 하드 차단 (planState 활성 시)
2. TodoWrite Enforcer (Plan Mode 비활성 시에만)
3. PreToolUse hook
4. 도구 실행
5. PostToolUse hook
```

---

## 8. TUI 상태 표시

| 상태 | 배지 |
|------|------|
| Plan Mode 꺼짐 | (없음) |
| Plan Mode 켜짐, 대기 없음 | `📋 PLAN` |
| Plan Mode 켜짐, N개 대기 | `📋 PLAN (N pending)` |

---

## 9. 파일 맵

| 파일 | 역할 |
|------|------|
| `internal/agent/planmode/mode.go` | State, Mode enum |
| `internal/agent/planmode/pending_queue.go` | PendingCommand, Queue |
| `internal/agent/planmode/transition.go` | Enter/Exit/Toggle |
| `internal/agent/planmode/readonly_filter.go` | Allow() FAIL-CLOSED |
| `internal/agent/planmode/approval.go` | Trigger(), ApprovalHandler |
| `internal/agent/todo/signals.go` | 신호어 사전 + DetectMultiStep() |
| `internal/agent/todo/prompt_injector.go` | InjectIfNeeded() |
| `internal/agent/todo/enforcer.go` | Enforce() |
| `internal/agent/query/tool_invoker.go` | 차단 순서 오케스트레이션 |
| `internal/tui/statusbar.go` | planPending 배지 |
| `internal/tui/app_key_handler.go` | Shift+Tab approval 트리거 |
| `cmd/infractl/plan.go` | plan enter/exit/status CLI |
