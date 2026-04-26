# InfraCtl Flow Diagrams

## 1-A. Prompt 조합 흐름

LLM에 보내는 시스템 프롬프트가 어떻게 만들어지는가.

```
 ┌─────────────────────────────────────────────────────────────┐
 │                    User Input (사용자 요청)                  │
 └──────────────────────────┬──────────────────────────────────┘
                            │
                            v
 ┌──────────────────────────────────────────────────────────────┐
 │  1. Classification (분류)  [classify.go]                     │
 │                                                              │
 │  Step A: 정규식 패턴 매칭 (LLM 호출 없이)                    │
 │    "안녕", "hi" → needs_tools=false → 간소 프롬프트로 빠짐   │
 │    "설치해줘"   → tierHint="reasoning"                       │
 │    "요약해줘"   → tierHint="fast"                            │
 │                                                              │
 │  Step B: LLM에 classify_request 도구 호출                    │
 │    → ClassifyResult {                                        │
 │        NeedsTools:     true,                                 │
 │        ToolGroups:     ["shell","file","system_info"],       │
 │        PromptSections: ["safety","behavior","tool_priority"],│
 │        Tier:           "general",                            │
 │        TaskType:       "configure"                           │
 │      }                                                       │
 └───────────────────────────┬──────────────────────────────────┘
                             │
              ┌──────────────┼──────────────┐
              v              v              v
 ┌──────────────┐ ┌──────────────┐ ┌──────────────────┐
 │ToolGroups    │ │PromptSections│ │Tier 선택         │
 │→ 허용 도구   │ │→ SectionSet  │ │reasoning/general │
 │  이름 필터   │ │  비트 플래그 │ │/fast 모델 결정   │
 └──────┬───────┘ └──────┬───────┘ └────────┬─────────┘
        │                │                   │
        └────────────────┴───────────────────┘
                         │
                         v
 ┌──────────────────────────────────────────────────────────────┐
 │  2. BuildContextualLayoutAt()  [prompt_layout.go]            │
 │                                                              │
 │  ┌─────────────── prefix ──────────────────┐                 │
 │  │ appendContextualCore():                  │                 │
 │  │   "You are infractl..." + 핵심 원칙     │                 │
 │  │   현재 날짜/시간                          │                 │
 │  │ appendContextualEnvironment():           │                 │
 │  │   활성 서버 컨텍스트 (OS/환경 정보)       │                 │
 │  └──────────────────────────────────────────┘                 │
 │                                                              │
 │  ┌──────── beforeKnowledge sections ───────┐                 │
 │  │ [조건부] Available Tools (Qwen 인덱스)  │                 │
 │  │ [조건부] Known Servers Pool              │                 │
 │  │ [조건부] 로컬 vs 원격 가드레일            │ ← 한글화 완료  │
 │  │ [조건부] Server Focus 가이드             │                 │
 │  └──────────────────────────────────────────┘                 │
 │                                                              │
 │  ┌──────── [Knowledge Slot] ───────────────┐                 │
 │  │ Task Memory (성공/실패 패턴)             │                 │
 │  │ RAG Knowledge Context (벡터 검색 결과)   │                 │
 │  └──────────────────────────────────────────┘                 │
 │                                                              │
 │  ┌──────── afterKnowledge sections ────────┐                 │
 │  │ [조건부] RAG Knowledge 우선순위          │                 │
 │  │ [조건부] Learned Systems (발견된 서비스) │                 │
 │  │ [조건부] Active Connectors (DB/앱)       │                 │
 │  │ [조건부] 안전 규칙                        │ ← 한글화 완료  │
 │  │ [조건부] 전용 도구 우선 사용              │ ← 한글화 완료  │
 │  │ [조건부] 도구 선택 가이드라인             │ ← 한글화 완료  │
 │  │ [조건부] 오류 복구 프로토콜               │ ← 한글화 완료  │
 │  │ [조건부] 서비스 자동 탐지 흐름            │ ← 한글화 완료  │
 │  │ [조건부] 작업 완료 규칙                   │ ← 한글화 완료  │
 │  │ [조건부] 행동 규칙                        │ ← 한글화 완료  │
 │  │ [조건부] INFRACTL.md (사용자 정의 규칙)  │                 │
 │  └──────────────────────────────────────────┘                 │
 └───────────────────────────┬──────────────────────────────────┘
                             │
                             v
 ┌──────────────────────────────────────────────────────────────┐
 │  3. layout.Render() → 최종 systemPrompt 문자열               │
 │     (모든 섹션을 하나의 문자열로 결합)                        │
 └──────────────────────────────────────────────────────────────┘
```

---

## 1-B. Agent Decision Loop

판단 / 선택 / 루핑 흐름.

```
 ┌─────────────────────────────────────────────────────────────┐
 │                    User Input (사용자 요청)                  │
 └──────────────────────────┬──────────────────────────────────┘
                            │
                 ┌──────────┴──────────┐
                 │   Agent.Run()       │
                 │   [loop.go]         │
                 │                     │
                 │ ・prefetchKnowledge │ ─── (비동기 병렬)
                 │ ・prefetchTaskMem   │
                 └──────────┬──────────┘
                            │
                            v
           ┌────────────────────────────────────┐
           │ ①  Classify (분류/판단)             │
           │    "이 요청에 도구가 필요한가?"     │
           │    "어떤 모델 티어를 쓸 것인가?"    │
           ├────────────────────────────────────┤
           │  needs_tools=false ─────────────────┼──→ 간소 LLM 호출 → 텍스트 응답
           │  needs_tools=true                  │
           └─────────────┬──────────────────────┘
                         │
                         v
           ┌────────────────────────────────────┐
           │ ②  Prompt 조합 (위 1-A 참조)       │
           │    시스템 프롬프트 + 히스토리 구성   │
           └─────────────┬──────────────────────┘
                         │
                         v
 ┌═══════════════════════════════════════════════════════════════╗
 ║  ③  query.Engine.Run()  [loop_engine.go + agent/query]        ║
 ║                                                               ║
 ║  ┌───────── for turn := 0; turn < MaxTurns; turn++ ──────┐   ║
 ║  │                                                        │   ║
 ║  │  A. Context 취소 확인                                  │   ║
 ║  │     └─ ctx.Err() != nil → Terminal{interrupted}       │   ║
 ║  │                                                        │   ║
 ║  │  B. EventStreamStart 발행                              │   ║
 ║  │                                                        │   ║
 ║  │  C. LLM 호출 (ChatStream)                              │   ║
 ║  │     ├─ token/thinking → EventAssistantChunk            │   ║
 ║  │     ├─ 성공 → D단계로                                  │   ║
 ║  │     └─ 실패 → EventError + Terminal{model_error}      │   ║
 ║  │                                                        │   ║
 ║  │  D. Assistant 응답 확정                                │   ║
 ║  │     └─ EventAssistantResponse                          │   ║
 ║  │                                                        │   ║
 ║  │  E. ToolCalls 판별                                    │   ║
 ║  │     ├─ 없음 → Terminal{completed}                      │   ║
 ║  │     └─ 있음 → F단계로                                  │   ║
 ║  │                                                        │   ║
 ║  │  F. StreamingExecutor 실행                             │   ║
 ║  │     ├─ PartitionToolCalls()                            │   ║
 ║  │     ├─ read-only batch 병렬                             │   ║
 ║  │     ├─ mutation batch 순차 + sibling skip              │   ║
 ║  │     ├─ 결과 완료 즉시 EventToolResult                  │   ║
 ║  │     ├─ 결과를 state/history에 추가                     │   ║
 ║  │     ├─ RAG 사후 주입                                  │   ║
 ║  │     └─ 루프 상단으로 복귀 ─────────────────────┐      │   ║
 ║  │                                                │      │   ║
 ║  └────────────────────────────────────────────────┘      │   ║
 ║                                                           │   ║
 ║  최대 반복 도달 → Terminal{max_turns}                    │   ║
 ╚═══════════════════════════════════════════════════════════════╝
```

---

## 1-C. 도구 선택 & 실행 파이프라인 (PreflightValidator 포함)

```
 LLM Response: ToolCalls[]
       │
       v
 ┌──────────────────────────────────────────┐
 │ partitionToolCalls()                     │
 │                                          │
 │ for each toolCall:                       │
 │   tool.IsReadOnly()?                     │
 │     YES → readOnly[]  (병렬 실행)        │
 │     NO  → mutation[]  (순차 실행)        │
 └──────┬────────────────────┬──────────────┘
        │                    │
        v                    v
  ╔═══════════╗       ╔═══════════════╗
  ║ readOnly  ║       ║ mutation      ║
  ║ 병렬 실행 ║       ║ 순차 실행     ║
  ║ WaitGroup ║       ║ fail → skip   ║
  ╚═════╤═════╝       ╚══════╤════════╝
        │                    │
        └─────────┬──────────┘
                  │
                  v
 ╔══════════════════════════════════════════════════════════════╗
 ║  executeSingleTool(ctx, toolCall)  [tool_exec.go]           ║
 ║                                                             ║
 ║  1. Tool 조회     registry.Get(name)                        ║
 ║  2. 인자 파싱     json.Unmarshal(args)                      ║
 ║  3. Target 결정   ExtractTarget → connectorMgr → activeServer║
 ║  4. Executor 획득 manager.Get(target) → local/SSH           ║
 ║  5. Hook 실행     hooksMgr.Fire(BeforeExecute)              ║
 ║  6. Checkpoint    checkpointMgr.CreateFromArgs()            ║
 ║  7. Idle 핸들러   wrapWithIdleDetect()                      ║
 ║                                                             ║
 ║  ┌─────────────────────────────────────────────────────┐    ║
 ║  │  ★ PreflightValidator.Validate()  ★                 │    ║
 ║  │  = LLM 미니 에이전트 루프 (Fast 티어, 최대 3라운드) │    ║
 ║  │  ※ mutation 도구에만 적용 (읽기 전용 도구는 건너뜀) │    ║
 ║  │                                                     │    ║
 ║  │  Round 1: LLM이 필요한 probe 도구 결정 & 병렬 실행  │    ║
 ║  │    probe_system_info  → OS/사용자/권한 수집          │    ║
 ║  │    probe_binary       → 바이너리 존재/버전           │    ║
 ║  │    probe_service      → 서비스 상태                  │    ║
 ║  │    probe_port         → 포트 사용 현황               │    ║
 ║  │    probe_disk         → 디스크 공간/권한             │    ║
 ║  │    probe_process      → 관련 프로세스 목록           │    ║
 ║  │    probe_command      → 임의 읽기 전용 명령          │    ║
 ║  │    probe_exec_history → 과거 실행 이력              │    ║
 ║  │                                                     │    ║
 ║  │  Round 2: LLM이 수집 결과 검토 후 판정              │    ║
 ║  │    선택A: validate_command 호출 → 최종 판정 반환     │    ║
 ║  │    선택B: 추가 probe 호출 (Round 3으로)              │    ║
 ║  │                                                     │    ║
 ║  │  Round 3 (필요 시): 최종 validate_command 호출      │    ║
 ║  │    (3라운드 도달 시 강제로 validate_command 호출)    │    ║
 ║  │                                                     │    ║
 ║  │  판정 결과:                                         │    ║
 ║  │   ╔═══════╗  → 실행 차단, 사유+대안을 LLM에 전달    │    ║
 ║  │   ║ BLOCK ║                                         │    ║
 ║  │   ╚═══════╝                                         │    ║
 ║  │   ┌───────┐  → TUI에 경고 표시 후 실행 계속          │    ║
 ║  │   │ WARN  │                                         │    ║
 ║  │   └───────┘                                         │    ║
 ║  │   ┌───────┐  → 검증 통과, 정상 실행                  │    ║
 ║  │   │ PASS  │                                         │    ║
 ║  │   └───────┘                                         │    ║
 ║  └─────────────────────────────────────────────────────┘    ║
 ║                                                             ║
 ║  8. tool.Execute(ctx, args, exec) → ToolOutcome             ║
 ║  9. recordExecLog() → SQLite 기록                           ║
 ╚═════════════════════════════════════════════════════════════╝
```

---

## 주요 패키지 구조

```
internal/
├── agent/
│   ├── loop.go              — Agent.Run() 진입점
│   ├── loop_engine.go       — query.Engine adapter, event 소비
│   ├── query/               — state machine, streaming executor, terminal reason
│   ├── tool_exec.go         — executeToolCalls(), executeSingleTool()
│   ├── prompt_layout.go     — BuildContextualLayoutAt(), buildMinimalChatAt()
│   ├── prompt.go            — appendBehaviorRules(), 서버 컨텍스트 섹션
│   ├── prompt_tools.go      — 도구 선택/안전/에러 복구/작업 완료 섹션
│   └── classify.go          — ClassifyRequest, ClassifyResult
│
└── preflight/               — Pre-Execution Validator
    ├── types.go             — Validator 인터페이스, ValidateReport 타입
    ├── validator.go         — LLMValidator (미니 에이전트 루프)
    ├── validator_prompt.go  — 검증 전용 시스템 프롬프트 (한글)
    ├── probe_tools.go       — 8개 읽기 전용 probe 도구 구현
    ├── probe_registry.go    — probe 도구 레지스트리
    ├── probe_safety.go      — probe_command 안전장치
    ├── tool_schema.go       — probe + validate_command JSON 스키마
    └── context_cache.go     — probe_system_info 5분 TTL 캐시
```
