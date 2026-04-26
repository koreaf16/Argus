# Phase 11 — TaskContext & Privilege Session Reuse

## Overview

Phase 11 adds two major capabilities:

1. **Task Context Manager** — Declarative task scope (target server, account, working directory, forbidden operations, plan steps) that is injected into every LLM turn so the agent never loses context mid-task.

2. **Privilege Session Reuse** — sudo/su passwords are asked once per session and cached in a TTL-scoped vault. Once root is acquired, LLM system prompt reflects current user so it stops prepending `sudo`.

## Architecture

### Key Packages

| Package | Responsibility |
|---------|---------------|
| `internal/agent/taskctx` | TaskContext, Manager, Snapshot, Guardrails, PendingProposal, templates |
| `internal/privilege` | ElevationState, Tracker, Vault |
| `internal/agent` | Loop proposal gate, TaskGuard, prompt injection |
| `internal/tools` | propose_task, declare_task, end_task, task_note tools |
| `internal/connector/ssh` | ElevationEventSink wiring in PersistentShell/SessionManager |
| `internal/tui` | ProposalCard, TaskPanel, statusbar Task summary |

### Flow

```
User: "Oracle 19c 설치해줘"
  ↓ classify.go: Complexity=complex, RequiresTaskProposal=true
  ↓ LLM calls propose_task(...)
  ↓ loop_proposal.go: PendingProposal activated
  ↓ TUI: RenderTaskProposalCard() displayed
User: "yes"
  ↓ loop.go Proposal Gate: executePendingProposal()
  ↓ taskMgr.Declare(req)  → TaskContext active
  ↓ Every turn: snap.RenderMarkdown() injected into system prompt
  ↓ shell_exec tool called → TaskGuard.Evaluate() checks Forbidden
  ↓ Elevation: sudo/su → PersistentShell.Elevate() → sink.OnElevationDone()
  ↓ Tracker records → next sudo prompt → Vault.Lookup() → auto-fill password
User: "끝났어"
  ↓ LLM calls end_task(status=completed, summary=...)
  ↓ Manager.End() → TaskContext moved to history
```

## Data Structures

### TaskContext
Stores the active task scope: server, account, workdir, forbidden patterns, plan steps, elevation reference.

### ElevationState
Per-session chain of user hops (admin→root→oracle), current user, method (sudo/su), timestamps.

### Vault
TTL-30min, session-scoped credential store. Wraps existing privilege.Cache. All slog calls omit password values.

## Security

- Vault passwords: never logged at any slog level
- TaskGuard separates from preflight.Validator — root elevation does NOT bypass preflight dangerous-command gate
- Forbidden patterns fail-closed: invalid regex rejects Declare

## Milestones

| M | Description | Status |
|---|-------------|--------|
| M1 | Data structures + tools package | Done |
| M2 | Complexity classification + Proposal flow | Done |
| M3 | Agent loop integration, TaskGuard, prompt injection | Done |
| M4 | PersistentShell elevation hooks, Vault in idle handler | Done |
| M5 | Web handler stub + docs | Done |
