# InfraCtl 아키텍처 개요

## 목적
`infractl`은 로컬 또는 원격 서버를 대상으로 대화형 운영 작업을 수행하는 Go 기반 AI CLI다.  
기본 실행은 로컬이고, 원격 대상은 SSH를 통해 다룬다.

## 핵심 원칙
- Local-first: 별도 서버 없이 로컬에서 바로 실행한다.
- Remote via SSH: 원격 서버는 SSH로만 다룬다.
- LLM-driven tools: 사용자의 요청을 해석해 도구를 고르고 실행한다.
- Safety first: 위험 작업은 단계적으로 확인한다.

## 저장소 개요
- 설정: `~/.infractl/config.yaml`
- 상태 관리: SQLite (`internal/store`)
- 핵심 로직: `internal/` (agent, executor, connector, discovery 등)
- CLI 인터페이스: `cmd/`
- 빌드 결과물: `bin/`

## 주요 패키지 책임 (internal/)
| 패키지 | 책임 | 재설계 후 (Phase A~G 완료 시) |
|---|---|---|
| `agent` | LLM 기반 메인 루프, 도구 실행 오케스트레이션 | `agent/query` (state machine + streaming), `agent/compact` (4중 stack), `agent/todo` (다단계), `agent/planmode` (승인 gate) 추가 |
| `executor` | 로컬/원격 명령어 실행 및 입출력 제어 | `executor/shell/{bash,powershell,analysis}` (ShellProvider + bash AST + PowerShell EncodedCommand) 추가, 정규식 위험 분석 제거 |
| `connector` | SSH 및 원격 환경 연결 관리 | 그대로 |
| `discovery` | 서비스 및 인프라 자원 식별 | 그대로 |
| `store` | 데이터 영속성 관리 (SQLite) | 그대로 |
| `tools` | 에이전트가 사용하는 기본 도구 세트 | `tools/todo_write.go`, `tools/monitor.go` 추가 + hook 통합 |
| `mcp` | Model Context Protocol 연동 | 그대로 |
| `subagent` | 복잡 작업 처리를 위한 위임 에이전트 | `subagent/parallel.go` (errgroup 기반 병렬), `subagent/isolation.go` 추가 |
| `tui` | 대화형 사용자 인터페이스 | Plan Mode 단축키 + 승인 UI 추가 |
| `web` | 대시보드 및 API 서버 구현 | 그대로 (Phase 7 별도 진행) |
| **`hooks`** (신설) | 정책 외부화 — 4 backend (command/prompt/http/agent) + fsnotify 핫리로드 | Phase A 골격 / D 활성 / G CLI |
| **`context`** (신설) | Query 진입 시 parallel fetch + DYNAMIC_BOUNDARY 캐시 분할 | Phase A |
| **`cache`** (신설/보강) | prefix_marker (prompt cache 최적화) | Phase A |
| **`background`** (신설) | 자동 백그라운딩 + 결과 파일화 + poller | Phase F |
| **`schedule`** (강화) | oneshot + retention + 로그 회전 | Phase F (기존 scheduler 강화) |
| (제거) `preflight` | validator / shell_precheck / structured_guard | Phase D 에서 hook 으로 흡수 후 제거 |

---

## 재설계 마이그레이션

InfraCtl 은 **claude_cli 패턴을 흡수**해 Query Engine / Compaction / Hook / Shell / 멀티태스크 / Plan Mode 를 완전 재설계 중이다.

- **차별점 보존**: LLM 분류 단계, Shell 창 표시 (PTY), Korean UX, circuit breaker, useInlineToolCalls
- **페이즈 구조**: A (인프라 골격) → B (Query Engine 교체) → (C 병렬 / D 병렬 / F 병렬) → E (Shell Provider 완성) → G (Plan Mode + CLI)
- **각 phase 별 실행 계획**: [`../docs_mig/`](../docs_mig/) — phase 별로 `claude_cli/src/` 원본 참조 소스 + 신설/수정 파일 + 검증 시나리오 기재
- **전체 비전**: [`design/redesign-overview.md`](design/redesign-overview.md)
- **Hook 시스템 상세**: [`design/hooks.md`](design/hooks.md)
- **Query Engine 상세**: [`design/query-engine.md`](design/query-engine.md)

**실행 규칙**: 각 phase 진입 전 사용자 질문 → 승인 → 별도 세션에서 코드 작업. 블라인드 진행 금지.

### Phase A 완료 (2025-04-17)

신설 패키지:
- `internal/lifecycle/` — 기존 SQLite lifecycle hooks 이관 (before_execute/after_execute/on_error/on_connect)
- `internal/hooks/` — claude_cli 스타일 hook 시스템 (PreToolUse/PostToolUse + 4 backend + hooks.yaml)
- `internal/context/` — boundary 마커, 환경 정보, INFRACTL.md 로드, errgroup 병렬 fetch
- `internal/cache/prefix_marker.go` — DYNAMIC_BOUNDARY 기반 prompt 분할 헬퍼
- `internal/executor/shell/` — ShellProvider 인터페이스 + bash/powershell 래퍼 골격
- `internal/agent/todo/` — TodoItem/Store/Tracker + TodoWrite/TodoRead LLM 도구

수정 파일: `tool_exec.go` (PostToolUse 삽입), `main.go` (hooks 부트스트랩), `wiring.go` (todo 도구 등록), `prompt.go` (TodoWrite 안내)

회귀: 기존 동작 전혀 변경 없음 (zero breakage).

### Phase B 완료 (2026-04-17)

Query loop를 `internal/agent/query` state machine으로 교체했다.

신설/변경:
- `internal/agent/query/` — `Engine`, `QueryEvent`, state/transition, partition, streaming executor, hook invoker 자리
- `internal/agent/loop_engine.go` — `Agent.Run()`과 query engine 사이의 adapter
- `internal/tui/query_adapter.go` — query event를 TUI message로 변환하는 sink
- `docs/design/query-engine.md` — Phase B 설계와 검증 매트릭스

제거:
- `internal/agent/loop_llm.go` — 기존 단순 LLM loop 제거

회귀:
- query golden 5개 통과
- Phase B E2E 8개 통과
- `go test ./...` 및 `go build -o bin\infractl.exe ./cmd/infractl/` 통과

### Phase C 완료 (2026-04-17)

4중 compaction stack + recovery 를 `internal/agent/compact/` 패키지로 신설했다.

신설 파일:
- `internal/agent/compact/types.go` — TokenState, CompactionResult, compact.State, 토큰 추정 함수
- `internal/agent/compact/budget.go` — 단일 tool_result 절단 (50K chars 상한)
- `internal/agent/compact/snip.go` — 오래된 turn 통째 LLM 요약 (자체 설계)
- `internal/agent/compact/breaker.go` — circuit breaker 래퍼 (임계=3, 쿨다운=5분)
- `internal/agent/compact/auto.go` — 토큰 상태 기반 proactive compaction (TokenWarning→mild, Critical/Overflow→aggressive)
- `internal/agent/compact/reactive.go` — PTL 1회 한정 reactive compaction (자체 설계)
- `internal/agent/compact/micro.go` — 오래된 tool_result 메시지 요약 (keepRecent=10 보존)
- `internal/agent/compact/collapse.go` — staged drain 우선순위 큐 (max-heap, 자체 설계)
- `internal/agent/compact/stack.go` — budget→snip→micro→collapse→auto 5단계 순차 파이프라인
- `internal/agent/compact/recovery.go` — PTL/max_output/media/fallback 에러 분기 (7 RecoveryAction)
- `internal/agent/compact/*_test.go` — 각 전략 단위 테스트 27개

수정 파일:
- `internal/agent/query/engine.go` — `compact *compact.Stack` 필드 + `SetCompact()` 메서드 추가
- `internal/agent/query/engine_dispatch.go` — 매 turn 진입 시 `compact.Stack.Apply` 호출 (stub 제거)
- `internal/agent/query/state.go` — `Params.ContextWindow`, `Params.SystemTokens` 필드 추가
- `internal/agent/loop_engine.go` — params 에 ContextWindow/SystemTokens 전달
- `internal/agent/agent_struct.go` — `compactStack *compact.Stack` 필드 + New() 초기화 + SetSessionSummary 에서 mild 연결

주요 설계 결정:
- `compact.State` 를 compact 패키지에 자체 정의 — query/agent 패키지와의 순환 의존 방지
- `MildCompactor` 인터페이스 — `compact→agent` 역방향 의존 방지, SessionSummaryManager 가 구현
- 자체 설계 3개(snip/reactive/collapse): feature-gated 로 claude_cli 원본 미존재, 호출부 + docs_mig 근거로 설계
- 토큰 버퍼 임계값 절댓값 사용 (OK≥20K, Warning 13~20K, Critical 3~13K, Overflow<3K) — docs_mig 의 "75/87/100%" 오류 보정

회귀:
- `go test ./internal/agent/...` 27개 추가 포함 전체 통과
- `go build -o bin/infractl.exe ./cmd/infractl/` 통과
- 기존 legacy `compaction.go` 유지 (loop.go 경로 보존, Phase C 에서는 제거 안 함)

### Phase D 완료 (2026-04-17)

`internal/preflight/*` 를 hook 시스템으로 완전 이관하고 디렉토리를 제거했다.

신설 파일:
- `internal/hooks/builtins/system_risk.sh` — 결정론 fast-path 차단 스크립트 (rm -rf 시스템경로, dd 블록디바이스, mkfs, fork bomb, chmod/chown 시스템경로, iptables -F, ufw disable). stdin JSON → stdout JSON (HookOutput).
- `internal/hooks/builtins/agent/shell_validator.md` — LLM prompt backend 용 shell 검증 프롬프트. Fast tier, temperature=0, `$ARGUMENTS` 치환. 즉시차단/차단/허용/과차단금지 4구간 정책 명시.
- `internal/hooks/builtins/assets.go` — `//go:embed` 내장 + `Unbox(dstDir)` 언박싱 API + `LookupPrompt(name)` ($BUILTIN: 해결).
- `internal/hooks/defaults/hooks.yaml.default` — embed 기본 정책 YAML. Bash/shell_exec 에 system_risk.sh(5s) + shell_validator(90s) 순차 적용.
- `internal/hooks/defaults/embed.go` — `DefaultHooksYAML()` embed 래퍼.
- `internal/infrainit/hooks.go` — `BootstrapHooks(cfgDir)`: 최초 실행 시 hooks.yaml 배포 (존재하면 보존), 매 실행 시 builtins/ Unbox (보안 패치 자동 갱신).
- `internal/agent/query/hook_metadata.go` — `ComputeMetadata(toolName, args)`: ReadOnly/DiskModifying/NetworkAccess/DangerScore 계산. 파이프/세미콜론 분리 후 첫 단어 allow-list 검사.
- `internal/agent/equivalence_test.go` — 55 케이스 동등성 테스트 (system_risk deny 20, allow 15, readonly 분류 10, pass-through 10).
- `testdata/equivalence/*.yaml` — 4개 파일, 55 수동 큐레이션 케이스.

수정 파일:
- `internal/hooks/runner.go` — `RunPreToolUse` 활성화 (기존 항상-allow 스텁 제거), fail-closed (backend 에러 → deny).
- `internal/agent/query/tool_invoker.go` — `Session`/`Metadata` 주입 활성화 (Phase B 스텁 제거).
- `internal/hooks/backend_prompt.go` — `$BUILTIN:<name>` 접두어 해결 로직 추가 (embed에서 읽음).
- `cmd/infractl/main.go` — `BootstrapHooks` 호출 추가, preflight 주입 제거.
- `internal/hooks/event.go` — `HookOutput` 스키마 교체: `Approved bool` → `Decision string (allow|deny|ask)` + `SystemMessage` + `NewInput`.

제거:
- `internal/preflight/` 전체 (10 파일: validator, shell_precheck, structured_guard, validator_prompt, probe_tools, probe_registry, probe_safety, tool_schema, context_cache, types).
- TUI preflight UX: `preflightBadge()`, `PreflightResultMsg`, `OnPreflightResult` 인터페이스 메서드, `preflightVerdict/Reason` 필드 — claude_cli 스타일 (hook deny → tool_result systemMessage) 로 통일.

주요 설계 결정:
- LLM 위임 정책: 800줄 결정론 shell_precheck 대신 fast-path .sh 1개 + LLM 프롬프트 1개로 슬림화.
- `ask` decision 은 현재 deny 로 처리 (Phase G Plan Mode 에서 양방향 승인으로 구현 예정).
- `SetEscapeHTML(false)`: JSON 인코딩 시 `>` → `\u003e` 방지 (shell grep 패턴 보존).
- hooks.yaml: 사용자 커스터마이즈 보존 (기존 파일 있으면 덮어쓰지 않음). builtins: 보안 패치를 위해 항상 갱신.

회귀:
- `go test ./...` 25 패키지 전부 통과 (equivalence 55/55 포함)
- `go build -o bin/infractl.exe ./cmd/infractl/` 통과

### Phase E 완료 (2026-04-17)

bash 파서(mvdan.cc/sh/v3) 기반 AST 분석 + PowerShell EncodedCommand 전환 + ShellProvider 통합 완료.

신설 파일:
- `internal/executor/shell/bash/parser.go` — AST 파싱 (`syntax.LangBash`)
- `internal/executor/shell/bash/heredoc.go` — heredoc 추출/복원 (quoted/unquoted 구분)
- `internal/executor/shell/bash/semantics.go` — FAIL-CLOSED AST 분석 (`CheckSemantics`): CallExpr/Pipe/CmdSubst/ProcSubst/Redirect/FuncDecl 방문
- `internal/executor/shell/bash/danger/{score,patterns}.go` — 위험도 체계(Critical 100/High 70/Medium 40/Low 10/UserApproval→High), CheckCallExpr/CheckPipe
- `internal/executor/shell/bash/readonly/{git,gh,docker,rg,posix,registry}.go` — 읽기전용 화이트리스트 (GIT/GH/DOCKER/RG/POSIX 300+ 명령)
- `internal/executor/shell/bash/specs/specs.go` — CommandSpec 레지스트리 (AutoFlags, Passthrough)
- `internal/executor/shell/bash/{quoting,pipe,snapshot}.go` — POSIX 인용, pipe stdin 재배치, env/pwd/alias/shopt 스냅샷
- `internal/executor/shell/powershell/{encode,quoting,snapshot}.go` — UTF-16LE base64 인코딩, PowerShell 인용, 환경 스냅샷
- `internal/executor/shell/powershell/specs/dangerous.go` — PowerShell 위험 패턴 (IEX/Remove-Item 등)
- `internal/executor/shell/analysis.go`, `prepared.go` — 공유 타입 (`Analysis`, `PreparedCmd`, `Level`, `Finding`)
- `internal/executor/shell/bash/semantics_test.go`, `equivalence_test.go` — AST 분석 단위 테스트 + 55 동등성 재검증
- `docs/design/shell-provider.md` — ShellProvider 설계 문서

수정 파일:
- `internal/executor/shell/{bash,powershell}/provider.go` — `Prepare()`/`Analyze()` 구현 + `init()` 자동 등록
- `internal/executor/local.go` — `buildCommand`: `InjectNonInteractiveFlags` 제거, `shell.Resolve()→Prepare()` 위임
- `internal/executor/{local_session,local_pty_unix,local_interactive_unix}.go` — `PreparedCmd.CleanupFns` 연결
- `internal/agent/query/hook_metadata.go` — `shellMetadata`: 문자열 휴리스틱 → `bash` provider `Analyze()` 위임
- `internal/tools/{shell_exec_privilege,shell_exec_become}.go` — `InjectNonInteractiveFlags` 호출 제거

제거:
- `internal/executor/executor.go::InjectNonInteractiveFlags` (+ regexp/fmt/strings 의존) — `bash/provider.go::applyAutoFlags` (specs 기반)로 이관

주요 설계 결정:
- FAIL-CLOSED 원칙: 미식별 CallExpr → `LevelUserApproval` (자동 차단이 아닌 사용자 승인 요청)
- Level 우선순위: Critical(5) > High(4) > UserApproval(3) > Medium(2) > Low(1) — iota 비교 불가
- `/dev/null` redirect: `IsReadOnly` 변경 없음 (safeRedirectTargets 체크 후 IsReadOnly 변경)
- 절대경로 명령(`/usr/bin/ls`): `handleCallExpr`에서 경로 프리픽스 제거 후 whitelist 조회
- PowerShell on Windows: `hook_metadata.go`는 인프라 명령(POSIX) 분석을 위해 항상 `bash` provider 사용
- 비대화형 플래그: `specs.Registry` AutoFlags 로 이관, `applyAutoFlags` (regex)로 bash `Prepare` 내부 적용

회귀:
- `go test ./...` 30 패키지 전부 통과 (equivalence 55/55 포함)
- `go build -o bin/infractl.exe ./cmd/infractl/` 통과

### Phase F 완료 (2026-04-18)

Background/Schedule/Subagent 다중 실행 레이어 완성.

신설 패키지:
- `internal/background/` — 자동 백그라운딩 + 결과 파일화 + poller
- `internal/schedule/` — oneshot + retention + 로그 회전 (기존 scheduler 강화)
- `internal/subagent/` — 서브에이전트 오케스트레이터 (Pre-flight intel 포함)

### Phase G 완료 (2026-04-18)

Plan Mode 하드 차단 + fsnotify 핫리로드 + CLI 서브커맨드 + TodoWrite enforcer 완성.

신설 패키지:
- `internal/agent/planmode/` — Mode enum, State, PendingQueue, ReadOnlyFilter, Approval (FAIL-CLOSED mutation 차단)
- `internal/hooks/watcher.go` — fsnotify 기반 hooks.yaml 감시 (500ms debounce, editor atomic save 대응)
- `internal/hooks/reloader.go` — yaml 재파싱 + atomic snapshot swap

신설/수정 파일:
- `cmd/infractl/hooks.go` — `infractl hooks {list,test,validate,reload}` 서브커맨드
- `cmd/infractl/plan.go` — `infractl plan {enter,exit,status}` 서브커맨드 + plan_state.json IPC
- `internal/agent/todo/signals.go` — 다단계 신호어 사전 (설치/배포/마이그레이션 등 10+)
- `internal/agent/todo/prompt_injector.go` — `InjectIfNeeded()` system prompt 주입
- `internal/agent/todo/enforcer.go` — mutation 도구 + todo 비어있으면 deny
- `internal/agent/query/tool_invoker.go` — Plan Gate + TodoEnforcer 체인 삽입
- `internal/tui/statusbar.go` — `📋 PLAN (N pending)` 배지
- `internal/tui/app_key_handler.go` — Shift+Tab 종료 시 pending 목록 표시

주요 설계 결정:
- Plan Mode: ReadOnlyFilter FAIL-CLOSED (미식별 도구 → deny)
- PendingQueue → Shift+Tab 종료 시 SystemMsg 로 표시, CLI `plan exit` 로 승인/거부
- TodoEnforcer: Plan Mode 비활성 시에만 작동 (Plan Mode 중 차단은 PendingQueue 로 처리)
- Windows: `hooks/config.go` 권한 체크 `runtime.GOOS != "windows"` 가드

회귀:
- `go test ./internal/agent/planmode/... ./internal/agent/todo/... ./internal/hooks/...` 전부 통과
- `go build ./...` 통과

마이그레이션 완료: Phase A–G 전부 구현. 다음 작업은 Phase 7 (Daemon+Web UI) 및 Phase 8 (모니터링).

