# Argus Architecture (Phase 3 — Context Graph)

## Runtime Flow
1. `cmd/argus/main.go` boots model registry, app state, memdir store, mcp/lsp/skills managers.
2. Tool registry is built with active Phase 2 tool surface.
3. `query.Engine` orchestrates LLM stream + tool call loop.
   - Internally maintains an **Episodic Context Graph** (`internal/context`) as the source of truth for conversation history.
   - Each turn: user/assistant/tool events → appended as `Node`s to the Graph.
   - Before each LLM call: `RenderForLLM()` projects Graph → `[]llm.Message` within the token budget.
4. `tui.Run` runs scrolling TUI: logo printed once to scrollback, each message emitted via `tea.Println`; only input+footer anchored at bottom via `View()`.

## Context Management Subsystem (`internal/context`)

### Architecture
```
User Input
    │
    ▼
Graph.AppendUser()          ← Episodic Context Graph (source of truth)
    │
    ▼
[LLM Stream] ──────────────────────────────────────► LLM API
    │                                                 ▲
    ▼                                                 │
Graph.AppendAssistant()                   RenderForLLM()
Graph.AppendToolUse()         ◄────────── (budget-aware projection)
    │
    ▼
Distiller.Distill(rawOutput)   ← FULL / PARTIAL / SUMMARY
    │    ├─ FULL (≤24k chars): inline 그대로
    │    ├─ PARTIAL (>24k):    head+tail + ArtifactStore 저장
    │    └─ SUMMARY (>32k):    extractive/LLM 요약 + ArtifactStore 저장
    ▼
Graph.AppendToolResult(inlineText, projection, artifactID)
```

### 파일 목록
| 파일 | 역할 |
|---|---|
| `node.go` | NodeKind 6종, Projection 4종, Node 타입 |
| `graph.go` | Graph: append/prune/protect/ForceConsolidate |
| `token_estimator.go` | chars→token 추정, budget 계산 (20% 출력 예약) |
| `renderer.go` | `RenderForLLM()`: Graph → `[]llm.Message` projection |
| `artifact.go` | ArtifactStore: raw output 파일 저장, ArtifactManifest |
| `distiller.go` | FULL/PARTIAL/SUMMARY 4단계 압축, 에러 보존 |

### 보호 규칙 (MarkProtected)
- 최신 `ProtectedTurns=2` 개 user turn과 해당 assistant 노드
- 마지막 tool_use + tool_result 쌍
- 현재 user 입력에 언급된 파일 경로를 포함한 tool_result
- plan/todo 관련 tool_use (TodoWrite, EnterPlanMode, ExitPlanMode)

### Budget 계산
```
budget = contextWindow - systemTokens - reservedOutput(20%) - currentUserTokens
```

### Context 초과 처리
1. CountTokens 결과 ≥ contextWindow → `ForceConsolidate()` (비보호 노드를 summary로 교체)
2. 재 RenderForLLM 후 LLM 호출 재시도
3. `/snip` 명령 → 동일하게 `ForceConsolidate()` 위임

## Session Snapshot

| Version | 형식 |
|---|---|
| v1 (기존) | `{"saved_at":..., "messages":[...]}` |
| v2 (신규) | `{"version":2, "saved_at":..., "messages":[...], "graph":[...], "artifacts":[...]}` |

v2는 `IsV2()` 메서드로 판별. v1 로드 시 `messages`에서 in-memory graph 재구성 가능 (migrate.go, 선택적).

## Artifact Store
```
~/.argus/session-artifacts/<sessionID>/<seq>-<tool>-<callID_prefix>.txt
```
- 세션별 디렉터리 독립
- raw output 전체 보존
- Graph에는 `artifact_id`, `artifact_path`만 참조로 유지

## Core Packages
- `internal/context`: Graph, Distiller, Renderer, ArtifactStore — context management subsystem.
- `internal/query`: conversation loop, tool execution, permission gating.
- `internal/tool`: tool interface and registry (including source-aware registration for MCP).
- `internal/tui`: scrolling TUI — transcript entries emitted via `tea.Println` to terminal scrollback; anchored area (input + footer) rendered each frame via `View()`.
- `internal/components/logo`: Argus ASCII icon + condensed session-start logo block.
- `internal/components/promptinput`: top-border-only input box and 1-line space-between footer renderer.
- `internal/repl`: legacy readline loop (fallback path via `--legacy-repl`).
- `internal/repl/commands`: slash command dispatcher and command handlers.
- `internal/state`: app/session/model/mcp/skills state helpers.
- `internal/memdir`: `~/.argus` file layout bootstrap and JSON persistence.
- `internal/services/mcp`: local MCP config loading, resource/tool listing, and bridge-tool facade.
- `internal/services/lsp`: language-server facade with process lifecycle and stdio JSON-RPC client.
- `internal/skills`: bundled + user-loaded skill registry and execution facade.

## Default Tool Surface
- Kept: `websearch`, `fileread`, `filewrite`, `glob`, `grep`, `webfetch`,
  `lsp`, `mcp`, `list_mcp_resources`, `read_mcp_resource`, `mcp_auth`, `skill`.
- MCP dynamic bridges: for each configured `mcp.json` tool entry, runtime registers
  `mcp__<server>__<tool>` via source-aware registry.
- Deferred from default runtime surface:
  `agenttool`, `askuserquestiontool`, `brieftool`, `configtool`,
  `enterworktreetool`, `exitworktreetool`, `fileedittool`, `powershelltool`,
  `repltool`, `schedulecrontool`, `sendmessagetool`, `sleeptool`,
  `syntheticoutputtool`, `task*`, `team*`, `remote*`, `notebookedittool`,
  `toolsearchtool`.
