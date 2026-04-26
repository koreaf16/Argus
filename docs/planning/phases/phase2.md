# Argus Phase 2 Rebaseline (2026-04-22)

## Goals
- Keep the root baseline green: `go test ./...`.
- Keep only Phase 2 tools on the default runtime surface.
- Move slash-command handling to `internal/repl/commands`.
- Provide practical platform facades for memory/session, MCP, LSP, and skills.

## Implemented
- Baseline cleanup
  - Removed case-collision `internal/tools/*Tool` directories.
  - Removed API-mismatched files:
    - `internal/bootstrap/state.go`
    - `internal/hooks/use_can_use_tool.go`
  - `infractl_참고용` remains isolated as a nested module.
- Tool/runtime plumbing
  - Source-aware registry APIs in `internal/tool/registry.go`.
  - `AppState` extensions for session ID, active MCP servers, and active skills.
  - Dynamic MCP bridge tool registration:
    - configured `mcp.json` tools become `mcp__<server>__<tool>`.
    - `/mcp reload` now syncs bridge tools (add/remove).
- Services
  - `internal/memdir`:
    - bootstrap for `~/.argus/{sessions,memory,todos,plans,worktrees,scheduled-tasks}`
    - atomic JSON save/load, memory list/search.
    - session todo/plan persistence helpers.
  - `internal/services/mcp`:
    - config load, server/tool/resource listing, resource read.
  - `internal/services/lsp`:
    - stdio JSON-RPC framing client
    - initialize/initialized/shutdown/exit flow
    - hover request support
    - diagnostics cache from `textDocument/publishDiagnostics`.
  - `internal/skills`:
    - bundled skills registry
    - user skill loading from `~/.argus/skills`
      (`*.skill.json` and markdown header `# skill: <name>`).
- REPL commands
  - Implemented: `/help`, `/model`, `/status`, `/session`, `/plan`,
    `/memory`, `/mcp`, `/skills`, `/commit`, `/diff`, `/review`,
    `/init`, `/config`, `/keybindings`.
  - `/commit`, `/diff`, `/review` return explicit errors outside git repos.
  - `/config` supports `show`, `get`, `set`.
  - `/init` in REPL creates missing baseline config files.
- Core execution hardening
  - Shared path guards and resolver helpers for local file tools.
  - `bash` and `powershell` moved from unconditional permission allow to policy-based allow/ask/deny.
  - `bash` and `powershell` now support validated `workdir` and execute in tool context working directory by default.
  - Removed shell credential-injection behavior from `bash` tool runtime.
  - `fileread` now supports line-range reads and root-boundary enforcement.
  - `filewrite` now uses atomic writes with root-boundary enforcement.
  - `glob` and `grep` now enforce root boundaries; `grep` prefers `rg` with Go fallback.
- Plan/todo behavior
  - `EnterPlanMode` now stores pre-plan mode and initializes session plan file.
  - `ExitPlanMode` now restores pre-plan mode, normalizes approved prompts (`bash`/`powershell`), and writes numbered approved steps to plan file.
  - `TodoWrite` now validates/persists structured todo lists and clears all-completed lists.
  - Engine/REPL now support multi-step plan execution:
    - engine emits `plan_execution_ready` from `ExitPlanMode` tool results.
    - REPL runs approved steps sequentially with per-step confirmation.
    - step execution syncs todo status (`pending`/`in_progress`/`completed`) per session.

## Default Tool Surface
- Included:
  - `websearch`, `fileread`, `filewrite`, `glob`, `grep`, `webfetch`
  - `lsptool`, `mcptool`, `listmcpresourcestool`,
    `readmcpresourcetool`, `mcpauthtool`, `skilltool`
- Deferred from default registration:
  - `agenttool`, `askuserquestiontool`, `brieftool`, `configtool`,
    `enterworktreetool`, `exitworktreetool`, `fileedittool`,
    `powershelltool`, `repltool`, `schedulecrontool`, `sendmessagetool`,
    `sleeptool`, `syntheticoutputtool`, `task*`, `team*`, `remote*`,
    `notebookedittool`, `toolsearchtool`.

## Verification
- `go test ./...` passes.
- Added tests:
  - `internal/memdir/store_test.go`
  - `internal/todostore/store_test.go`
  - `internal/tool/registry_test.go`
  - `internal/query/plan_execution_test.go`
  - `internal/tools/exitplanmode/exitplanmode_test.go`
  - `internal/repl/plan_steps_test.go`
  - `internal/repl/commands/dispatcher_test.go`
  - `internal/services/mcp/manager_test.go`
  - `internal/services/lsp/client_test.go`
  - `internal/services/llm/toolcalls_test.go`
  - `internal/skills/registry_test.go`

## Remaining Phase 2+ Work
- Replace MCP config-backed facade calls with official MCP SDK transport/auth flows.
- Extend LSP manager for more request routing beyond hover and diagnostics.
- Expand skill metadata/frontmatter behavior and richer executors.
