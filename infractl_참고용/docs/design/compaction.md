# compaction.md — 4중 Compaction Stack + Recovery 설계

## 1. 개요

InfraCtl 은 Phase C 에서 기존 2단계 (mild/aggressive) compaction 을 **4중 stack + recovery** 로 확장한다.

```
Stack.Apply() 호출 순서 (매 turn 진입 시):
  1. budget   — tool_result 단위 절단 (토큰 폭탄 방지)
  2. snip     — 오래된 turn 통째 요약 (배경 잡음 제거)
  3. micro    — 오래된 tool_use_id 메시지 요약 (캐시 친화)
  4. collapse — 대형 메시지 staged queue 적재 (점진 압축)
  5. auto     — 토큰 상태에 따른 proactive compaction (기존 mild/aggressive 후계)
```

에러 발생 시 `Recovery.Handle()` 이 PTL / max_output / media / fallback 분기 결정.

---

## 2. 토큰 버퍼 임계값 (보정)

> claude_cli `autoCompact.ts:62-65` 기준. 원래 docs_mig/03_phase_c_compaction.md §5 C.3 의 "75/87/100%" 표기는 실제 구현과 다름.

| 상태 | 조건 | 동작 |
|---|---|---|
| OK | remaining ≥ 20K | noop |
| Warning | 13K ≤ remaining < 20K | mild compaction (SessionSummaryManager) |
| Critical | 3K ≤ remaining < 13K | aggressive compaction (전체 요약 + trim) |
| Overflow | remaining < 3K | aggressive + 즉시 trimHistory |

```
remaining = contextWindow - estimateTokens(state.messages) - systemPromptTokens
```

기본 contextWindow: `maxContextTokens` (Agent.SetMaxContextTokens 로 주입, 기본 128K)  
reservedOutputTokens: 8K (기존 compaction.go 와 동일)

---

## 3. circuit breaker

- 임계값: `MAX_CONSECUTIVE_AUTOCOMPACT_FAILURES = 3` (claude_cli `autoCompact.ts:70` 기준)
- cooldown: 5분 (기존 `agent_struct.go:101` 유지)
- 구현: `compact/breaker.go` — `llm.CircuitBreaker` 래핑 + `Breaker` 인터페이스

---

## 4. 전략별 설계

### 4.1 budget (C.1)

- 대상: 단일 tool_result 가 큰 경우 (예: `cat large_file.txt`)
- 임계: 기본 50K chars (한 tool_result 단위)
- 전략: 가장 큰 tool_result 부터 절단, 끝에 `[truncated: N chars removed]` 마커 삽입
- claude_cli 참조: `query.ts:379` 호출부, `utils/toolResultStorage.ts` 실제 정의

### 4.2 snip (C.1)

- 대상: 오래된 turn 전체 (최근 keepRecentTurns 이전)
- 임계: 기본 keepRecentTurns=20
- 전략: 해당 turn 을 Fast tier LLM 으로 1-3 문장 요약 후 교체
- ★ 자체 설계: claude_cli `snipCompact.js` feature-gated 미존재 → `query.ts:403` 호출부 + docs_mig §5 C.1 근거

### 4.3 micro (C.5)

- 대상: 오래된 tool_use_id 단위 메시지 (최근 keepRecent 개 이전)
- 임계: 기본 keepRecent=10
- 전략: 해당 tool_use_id 쌍 (tool_call + tool_result) 을 요약 텍스트로 교체
- 캐시 친화: prefix_marker boundary 를 침범하지 않음 (Phase A `cache/prefix_marker` 연동)
- claude_cli 참조: `microCompact.ts:253 microcompactMessages`, `query.ts:870-892`

### 4.4 collapse (C.6)

- 대상: Warning 이상일 때 대형 메시지 (우선순위: 가장 큰 메시지부터)
- 전략: 즉시 압축하지 않고 staged queue 에 적재 → 매 turn drain 1개
- HasStaged() → recovery 에서 PTL 발생 시 drain 우선 시도
- ★ 자체 설계: claude_cli `contextCollapse/` feature-gated 미존재 → `query.ts:441, 1094-1116` 호출부 + `grouping.ts` 근거

### 4.5 auto (C.3)

- 기존 `compaction.go:compactIfNeeded` 의 후계
- Warning → mild (SessionSummaryManager.UpdateIfNeeded 호출, 기존 그대로)
- Critical/Overflow → aggressive (전체 요약 + trim)
- breaker 보호: `Allow()=false` → skip + trimHistory 폴백
- claude_cli 참조: `autoCompact.ts:93 calculateTokenWarningState`, `autoCompact.ts:241 autoCompactIfNeeded`

### 4.6 reactive (C.4)

- 발동 조건: PTL 413 에러 발생 + `state.hasAttemptedReactiveCompact=false`
- 전략: aggressive 요약 + media strip (이미지 base64 제거)
- 1회 한정 (`hasAttemptedReactiveCompact` 플래그)
- 실패 시 trim 폴백 (가장 오래된 tool_result 10개 삭제)
- ★ 자체 설계: claude_cli `reactiveCompact.js` feature-gated 미존재 → `query.ts:1119-1166` 호출부 근거

---

## 5. recovery 분기표

| 에러 | 조건 | 액션 |
|---|---|---|
| PTL 413 | collapse.HasStaged() | ActionDrainCollapse (drain 1개 후 retry) |
| PTL 413 | !state.hasAttemptedReactiveCompact | ActionReactiveCompact |
| PTL 413 | 이미 reactive 시도 | ActionSurfaceError |
| max_output_tokens | !state.TriedMaxTokens64k | ActionRetryMaxTokens (64K) |
| max_output_tokens | 이미 시도 | ActionMultiTurnRecovery |
| media error | — | ActionStripMedia |
| fallback (5xx, overloaded) | — | ActionStripSignaturesAndRetry |
| 그 외 | — | ActionSurfaceError |

> `state.hasAttemptedReactiveCompact`, `maxOutputTokensRecoveryCount`, `maxOutputTokensOverride` 는 이미 `query/state.go` 에 정의됨 (Phase B 준비).

---

## 6. 기존 코드와의 관계

| 기존 | Phase C 처리 |
|---|---|
| `agent/compaction.go` | compact/* 로 분할 이전 후 deprecated → 제거 (C.9 완료 후) |
| `agent/session_summary.go` | 유지 (SessionSummaryManager). auto.go 가 UpdateIfNeeded 호출 |
| `agent_struct.go:compactBreaker` | compact/breaker.go 이전 후 agent_struct 에서 compact.Stack 주입으로 교체 |
| `agent/compaction.go:estimateTokens` | compact/types.go 또는 auto.go 로 이전 |
| `agent/history.go:groupByApiRound, flattenRounds` | compact 패키지에서 재사용 (import agent 대신 공통 함수로 이전 고려) |

---

## 7. 파일 목록 (내부 순서)

```
internal/agent/compact/
├── types.go      — TokenState enum, CompactionResult struct, estimateTokens
├── budget.go     — Budget interface, budgetApplier
├── snip.go       — Snip interface, snipApplier
├── breaker.go    — Breaker interface, NewBreaker (llm.CircuitBreaker 래핑)
├── auto.go       — Auto interface, autoCompactor (CalculateTokenState, Apply)
├── reactive.go   — Reactive interface, reactiveCompactor
├── micro.go      — Micro interface, microCompactor
├── collapse.go   — Collapse interface, collapseCompactor (staged queue)
├── stack.go      — Stack struct, NewStack, Apply
├── recovery.go   — Action enum, Recovery struct, Handle
└── *_test.go
```

---

## 8. 의존성

- 신규 외부 의존성 없음 (Phase C 범위)
- `internal/llm` — CircuitBreaker, Message, Client (요약 LLM 호출)
- `internal/agent/query` — state, Params (compact 패키지가 query 패키지를 참조하지 않도록 설계: state 대신 compact.State 도입 고려)

---

## 끝.
