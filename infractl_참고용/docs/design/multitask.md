# Multitask Design — 백그라운드 / Monitor / Subagent 병렬 / 스케줄

본 문서는 InfraCtl의 멀티태스크 서브시스템(Phase F)을 다룬다.
장기 실행 작업의 자동 백그라운딩, 실시간 출력 폴링, 서브에이전트 병렬 실행,
스케줄 고도화(oneshot / retention / log rotation)를 포함한다.

> 상태: **작업 중** (Phase F 진행). 각 섹션은 PR 단위로 채워진다.
> - 실행 계획: [../../docs_mig/06_phase_f_multitask.md](../../docs_mig/06_phase_f_multitask.md)
> - 아키텍처 총괄: [../infractl-architecture.md](../infractl-architecture.md)

---

## 1. Overview

InfraCtl의 이전 구조는 다음 격차를 갖고 있었다:

- `is_background` 파라미터 선언만 있고 promotion 로직 없음 — 장시간 명령이 foreground를 막음
- subagent 병렬이 `sync.WaitGroup` 기반 — sibling cancel 없음, yield-as-ready 없음
- 백그라운드 stdout/stderr 파일화 없음 → 결과가 메모리에만
- Monitor 도구 부재 → 진행 중 작업 출력 폴링 불가
- schedule에 oneshot / retention / log rotation 없음

Phase F는 `claude_cli`의 BashTool/TaskOutput/AgentTool/CronCreateTool 패턴을 흡수해 위 격차를 해소한다.

**사용자 체감 변화:**
- 30s 초과 명령은 자동으로 백그라운드 전환 → 다른 작업 병행 가능
- 5-10 서버 동시 SSH → subagent 병렬 (직렬 대비 3x+ 속도)
- `monitor` 도구로 진행 중 작업 출력 실시간 확인
- 스케줄 로그 회전 + oneshot 자동 정리

---

## 2. Background Lifecycle

```
foreground 실행 (tool_exec.go)
    │
    ├─[is_background=true]──────────────────────────► SubmitStreaming()
    │                                                    │
    ├─[promotableTools + threshold>0]                    │
    │   │  tryPromotion() goroutine 시작                 │
    │   ├─[완료 < threshold]── 동기 결과 반환            │
    │   └─[완료 ≥ threshold]── RegisterPending() ───────►│
    │                           goroutine 계속 실행       │
    │                           complete() on finish      │
    │                           "promoted to #N" 반환     │
    │                                                     ▼
    └─[default]── 동기 실행 후 반환          bgManager.jobs[id]
                                                    │
                                         ┌──────────┴──────────────┐
                                    Streamed=true              Streamed=false
                                    StoragePath 유효            메모리 only
                                         │                         │
                                    PollStdout()           Get(id).Result
                                    monitor / background_output    │
                                                         background_jobs result
```

**상태 전이:** `StatusRunning` → `StatusCompleted` | `StatusFailed` | `StatusCancelled`

---

## 3. Auto-Promotion Threshold

**값:** 30s (기본). `config.yaml` `background.promotion_threshold` 로 override.

**선택 근거:**
- `claude_cli`는 15s 사용 (web/build 플로우 전제).
- InfraCtl은 ops 작업 특성상 `apt update`, `tar`, 서비스 `restart` 등이 20–40s 대역에 빈번 → 15s는 과도한 promote 유발.
- 30s는 "평범한 ops 명령"은 foreground 유지, "정말 오래 걸리는 작업"만 promote하는 타협점.

---

## 4. File Storage Layout

**경로:** `~/.infractl/bg/<job_id>.{stdout,stderr,status}`

| 파일 | 용도 | 권한 |
|---|---|---|
| `<id>.stdout` | 표준 출력 스트리밍 | 0600 |
| `<id>.stderr` | 표준 에러 스트리밍 | 0600 |
| `<id>.status` | 완료 상태 (`status:<running\|completed\|failed\|cancelled>\nerror:<msg>`) | 0600 |

**회전 정책:** 파일이 10MB(기본) 이상이면 `.1`, `.2`, ... `.9` 순으로 이동 (최대 9단).

**정리 정책:** `retention.keep_days=7`, `retention.max_results=100` 초과 시 오래된 항목부터 삭제.
(`bgManager.CleanStorage(keepDays, maxResults)` 호출 — 현재는 수동; PR-4 스케줄 연동 예정)

**구현 파일:**
- `internal/background/storage.go` — `openJobFiles`, `rotateFile`, `writeStatusFile`, `cleanOldFiles`
- `internal/background/poller.go` — `PollStdout(storageDir, id, offset, maxBytes)`

---

## 5. Monitor Tool Semantics

**출력 제한:**
- **per-call**: 8 KB (Monitor 1회 호출 반환 최대)
- **cumulative**: 64 KB (한 Job에 대해 Monitor가 총 yield한 누적량)

**초과 시 동작:**
- per-call 초과 → 초과분 드롭 + `[truncated]` 접미사 출력
- cumulative 초과 → 이후 폴링은 새 데이터 없이 `[done]` / `[timeout]` 만 반환

**폴링 파라미터:**

| 파라미터 | 기본값 | 설명 |
|---|---|---|
| `job_id` | — | 필수. 폴링할 백그라운드 작업 ID |
| `offset` | 0 | 이전 호출에서 받은 `next_offset` 값 (증분 폴링) |
| `poll_interval_ms` | 1000 | 폴링 간격 (최소 100ms 클램핑) |
| `max_duration_ms` | 30000 | 단일 호출 최대 지속 시간 |

**반환 포맷:**
```
job_id=5 offset=0 next_offset=1024 done=false
... stdout 출력 내용 ...
[timeout]
```
접미사: `[done]` (작업 완료) / `[truncated]` (per-call 초과) / `[timeout]` (max_duration 초과) / `[cancelled]` (ctx 취소)

**구현 파일:**
- `internal/tools/monitor_tool.go` — `MonitorTool.Execute()`: PollStdout 반복 호출, cumulative 추적
- `internal/tools/output_tool.go` — `OutputTool.Execute()`: 단발성 offset 기반 읽기

---

## 6. Subagent Parallel Model

**동시성 모델:**
- `golang.org/x/sync/errgroup` 기반. `g.SetLimit(cfg.Subagent.MaxParallel)` 기본 10.
- 첫 에러 발생 시 공유 ctx 취소 → 진행 중 sibling이 다음 `client.Chat`에서 `ctx.Done()` 감지 → 조기 종료.
- `RunOpts.ContinueOnError=true`: sibling cancel 비활성화 (부분 실패 허용).

**격리 규칙:**
- 각 자식은 독자 ctx (errgroup 공유 ctx → parent 파생), Runner 얕은 복사, 독립 `eventCb`.
- 자식 간 event callback race 없음 (runner 얕은 복사 + `localRunner.eventCb = opts.EventCb` 독립 할당).
- 자식 간 tool registry / LLM history 공유 없음.

**`RunOpts` 구조체:**
```go
type RunOpts struct {
    MaxParallel     int           // 0 = 무제한; > 0 이면 errgroup.SetLimit(n)
    ContinueOnError bool          // false(기본): sibling cancel / true: 부분 실패 허용
    EventCb         EventCallback // 실시간 이벤트 콜백, nil 허용
}
```

**호출 예시 — Orchestrator 경유 (권장):**
```go
orch := subagent.NewOrchestrator(runner)
orch.SetMaxParallel(cfg.Subagent.MaxParallel)  // config 주입
results := orch.AnalyzeParallel(ctx, "db01", "디스크 사용량?",
    []subagent.AgentType{subagent.AgentTypeDB, subagent.AgentTypeOS})
```

**호출 예시 — RunParallel 직접 (저수준):**
```go
configs := []subagent.SubagentConfig{
    {Type: subagent.AgentTypeDB, Server: "db01", Question: q},
    {Type: subagent.AgentTypeOS, Server: "db01", Question: q},
}
opts := subagent.RunOpts{MaxParallel: 5, ContinueOnError: false}
results := subagent.RunParallel(ctx, configs, opts, runner)
// results[0] → DB, results[1] → OS (입력 순서 보장)
```

**Orchestrator 완전 교체 migration 노트:**
- 이전 구조: `AnalyzeParallelWithEvents` 내부에서 `sync.WaitGroup` + `make([]SubagentResult, n)` 직접 사용.
- 변경 후: `sync` 임포트 삭제. `RunParallel(ctx, configs, opts, o.runner)` 단일 호출로 위임.
- 외부 API(`AnalyzeParallel`, `AnalyzeParallelWithEvents`) 시그니처 불변 → 호출자 변경 없음.
- `SetMaxParallel(n int)` 추가 — `cmd/infractl/main.go`에서 config 주입.

---

## 7. Yield-as-Ready Result Collector

**목적:** 병렬 실행 중 완료된 subagent 결과를 기다리지 않고 즉시 소비자에게 전달.
입력 순서 보장이 필요 없는 스트리밍 UI / 이벤트 뷰어 등에 사용.

**채널 패턴:**
```go
ch := subagent.RunParallelStream(ctx, configs, opts, runner)
for r := range ch {   // 채널이 닫히면 루프 종료
    fmt.Printf("[%s] %s\n", r.Type, r.Answer)
}
```

**완료 순서 vs 입력 순서:**

| 함수 | 순서 | 채널 | 사용 시점 |
|---|---|---|---|
| `RunParallel` | 입력 순서 보장 | 없음 (슬라이스 반환) | 후처리 시 인덱스 필요 |
| `RunParallelStream` | 완료 순서 (빠른 것부터) | `<-chan SubagentResult` | 스트리밍 UI, 진행 표시 |

**내부 구조:**
- 버퍼 크기 = `len(configs)` — goroutine이 `ch <- r` 에서 절대 블로킹되지 않음.
- 모든 goroutine 완료 후 `defer close(ch)` → 소비자 for-range 자동 종료.
- ctx 취소 시: `select { case ch <- r: / case <-gCtx.Done(): return gCtx.Err() }` → 취소된 경우 결과 드롭.

**Progress 알림 패턴 (EventCb 활용):**
```go
opts := subagent.RunOpts{
    MaxParallel: 10,
    EventCb: func(e subagent.Event) {
        // 각 subagent 완료마다 호출
        log.Printf("[%s] 완료 (%dms)", e.AgentType, e.DurationMs)
    },
}
for r := range subagent.RunParallelStream(ctx, configs, opts, runner) {
    display(r)
}
```

---

## 8. Schedule Oneshot Semantics

**동작:**
- `OneshotManager.Add(ctx, at, name, prompt)`: `store.Schedule{Oneshot:true, RunAt:&at}` 저장 후 `time.AfterFunc(delay, run)`.
- `run()` 완료 시: `UpdateLastRun` 후 `SetScheduleEnabled(false)` — row 삭제하지 않고 비활성화.
  - 삭제는 `PruneOneshotSchedules`에 위임 (keep_days / max_results).
- 프로세스 재시작 시: `RestoreFromStore()` — `Oneshot=true && Enabled=true` 항목 전체를 재로드 후 타이머 재등록.
  - 과거 `run_at`: `delay=0` → 즉시 실행 (지나간 oneshot 폐기하지 않음).

**DB 스키마 변경:**
```sql
-- 기존 schedules 테이블에 idempotent ALTER TABLE (오류 무시)
ALTER TABLE schedules ADD COLUMN oneshot INTEGER NOT NULL DEFAULT 0;
ALTER TABLE schedules ADD COLUMN run_at DATETIME;
```

**완료 흐름:**
```
Add(at) → SaveSchedule → time.AfterFunc(delay, run)
                              │
                        run() 실행
                              │
                         UpdateLastRun
                              │
                    SetScheduleEnabled(false)  ← 비활성화 (완료 마킹)
                              │
                  PruneOneshotSchedules()  ← 보관 정책 적용 (별도 호출)
```

**구현 파일:** `internal/schedule/oneshot.go` — `OneshotManager` + `internal/schedule/retention.go` — `PruneOneshotSchedules`

---

## 9. Schedule Log Rotation

**정책:**
- 경로: `~/.infractl/schedule.log` (기본, config `schedule.log.path` override)
- 회전 임계: 100 MB (`schedule.log.max_size`)
- 회전 방식: sequential `.1`→`.2`→...→`.9` (최대 9단)
- Write 직전 파일 크기 체크 → 초과 시 즉시 회전 후 새 파일에 append

**크리덴셜 마스킹 정규식:**

| 패턴 | 예시 입력 | 치환 결과 |
|---|---|---|
| `password\s*[=:]\s*\S+` | `password=secret` | `password=***` |
| `passwd\s*[=:]\s*\S+` | `passwd: abc` | `passwd: ***` |
| `token\s*[=:]\s*\S+` | `token=eyJ...` | `token=***` |
| `api[_-]?key\s*[=:]\s*\S+` | `api_key=xyz` | `api_key=***` |
| `secret\s*[=:]\s*\S+` | `secret=pw` | `secret=***` |
| `auth\s*[=:]\s*\S+` | `auth=mytoken` | `auth=***` |

**로그 항목 포맷:**
```
[2026-04-17T16:51:24+09:00] schedule=job1 status=ok
prompt: df -h /data
result: Filesystem ... 45% /data
---
```
status는 `ok` / `error`. 오류 시 `error: <masked message>` 줄 추가.

**구현 파일:** `internal/schedule/log.go` — `Logger` / `internal/schedule/masker.go` — `MaskCredentials`

---

## 10. Hook Agent Backend

**mini-agent → HookOutput 매핑:**

| mini-agent 반환 | HookOutput 필드 | 결정 기준 |
|---|---|---|
| JSON `{"approved":true,"reason":"..."}` | `Approved=true`, `Reason="..."` | JSON 파싱 우선 |
| JSON `{"approved":false,"reason":"..."}` | `Approved=false`, `Reason="..."` | JSON 파싱 우선 |
| 텍스트에 deny/block/reject/거부/차단 포함 | `Approved=false`, `Reason=answer[:200]` | 텍스트 휴리스틱 |
| 기타 텍스트 (+ toolUses>0) | `Approved=true`, `Reason=answer[:200]` | 기본: 허용 |
| `err != nil` | — | 에러 상위 전파 |

**순환 import 회피:**
- `hooks.AgentRunner` 인터페이스는 `internal/hooks/agent_runner.go` 에 정의 (소비자 패키지 원칙).
- `internal/subagent.HookAgentRunner` → `hooks.AgentRunner` 구현 (`subagent` → `hooks` 단방향).
- `hooks` 패키지는 `subagent`를 직접 임포트하지 않는다.

**주입 흐름:**
```
main.go
  subRunner := subagent.NewRunner(...)
  hookRunner := hooks.NewRunner(llmRegistry)
  hookRunner.SetAgentRunner(subagent.NewHookAgentRunner(subRunner))
  ag.SetHookRunner(hookRunner)
```

**실행 흐름 (agent type=BackendAgent):**
```
runSingle(ctx, def, input)
  → selectBackend() → &agentBackend{runner: r.agentRunner}
  → agentBackend.Run(ctx, def, input)
      → buildAgentPrompt(def, input)  // $ARGUMENTS 치환
      → runner.RunForHook(ctx, prompt)
          → subagent.Runner.Execute(ctx, SubagentConfig{Type: AgentTypeIntel, ...})
      → parseAgentAnswer(answer, toolUses)
          → JSON 파싱 시도 → 실패 시 텍스트 휴리스틱
  → HookOutput{Approved, Reason}
```

**구현 파일:**
- `internal/hooks/agent_runner.go` — `AgentRunner` 인터페이스
- `internal/hooks/backend_agent.go` — `agentBackend.Run()`, `buildAgentPrompt`, `parseAgentAnswer`
- `internal/subagent/hooks_adapter.go` — `HookAgentRunner` 어댑터

---

## 11. Configuration

**`~/.infractl/config.yaml` 스키마:**

```yaml
background:
  promotion_threshold: 30s         # Duration string
  storage_dir: ~/.infractl/bg      # 결과 파일 디렉토리
  max_file_size: 10485760          # 10 MB
  retention:
    keep_days: 7
    max_results: 100

subagent:
  max_parallel: 10                 # errgroup.SetLimit

monitor:
  max_per_call: 8192               # 8 KB
  max_cumulative: 65536            # 64 KB per-job

schedule:
  retention:
    keep_days: 7
    max_results: 100
  log:
    path: ~/.infractl/schedule.log
    max_size: 104857600            # 100 MB
```

**환경 변수 override:**
- `INFRACTL_BG_THRESHOLD`: `30s`, `1m` 등 (기본: config의 `promotion_threshold`)
- 기타는 config.yaml 편집으로만 (Phase F 범위에서는 env override 최소화)

---

## 12. Migration Notes

**삭제 / 교체 대상:**
- `internal/subagent/orchestrator.go`의 `sync.WaitGroup` 경로 → `RunParallel` 직접 호출로 교체 (shim 유지하지 않음)
- 호출자 4곳 (예정) 모두 변경 — 삭제 전 사용자 확인

**하위호환:**
- `internal/background/manager.go` 기존 `Submit(ctx, desc, fn)` 유지 — memory-only result 호출자 (RAG 등) 보호.
- 새 `SubmitStreaming(ctx, desc, fn(ctx, *JobStreams))` 추가.
- `Job`에 `StoragePath string`, `Streamed bool`, `MonitorBytesEmitted int` 추가 (기존 필드 유지).

**DB 스키마 변경:**
- `schedules` 테이블 `oneshot INTEGER`, `run_at TEXT` 컬럼 추가 (ALTER TABLE, idempotent).

---

## 끝.
