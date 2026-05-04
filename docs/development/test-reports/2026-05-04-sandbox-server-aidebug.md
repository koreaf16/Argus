# sandbox-server `--aidebug` 50-case validation report

- Date: 2026-05-04
- Environment: `Argus.exe` on `C:\Dev\Argus`
- Target: `sandbox-server` SSH workspace
- Model: `models-gemma-4-26b-a4b-it-awq-4bit` (`/models/gemma-4-26B-A4B-it-AWQ-4bit`)
- Run mode: `--aidebug --auto-approve`

## Scope

I ran 50 prompts against `sandbox-server` in 5 batches:

- Batch 1: status, metrics, connection, and basic commands
- Batch 2: file listing, file reading, search, and directory counting
- Batch 3: destructive/write operations
- Batch 4: copy, permissions, service, and identity checks
- Batch 5: stress/output-format prompts

Captured artifacts:

- `scratch/aidebug-valid-runs/batch1/summary.json`
- `scratch/aidebug-valid-runs/batch2/summary.json`
- `scratch/aidebug-valid-runs/batch3/summary.json`
- `scratch/aidebug-valid-runs/batch4/summary.json`
- `scratch/aidebug-valid-runs/batch5/summary.json`

## Summary

| Metric | Value |
| --- | ---: |
| Total tests | 50 |
| Non-zero exit codes | 0 |
| Timeouts | 0 |
| Runs with `assistant.final` | 49 |
| Runs missing `assistant.final` | 1 |
| Runs with `tool.call.start` | 44 |
| Runs without `tool.call.start` | 6 |
| Average duration | 18.8 s |
| Median duration | 12.2 s |
| p90 duration | 31.6 s |
| p95 duration | 44.5 s |

Batch-level timing:

| Batch | Avg | Max |
| --- | ---: | ---: |
| batch1 | 12.3 s | 33.5 s |
| batch2 | 30.7 s | 76.8 s |
| batch3 | 26.3 s | 107.5 s |
| batch4 | 12.4 s | 30.8 s |
| batch5 | 16.4 s | 58.9 s |

## LLM Response Issues

These are the main response-quality problems observed across the 50 runs:

- 1 run ended without `assistant.final`.
- 5 runs had no `llm.tool_use` entry in the trace.
- 6 runs had no `tool.call.start` entry.
- Several underspecified prompts were answered as clarification requests instead of executing tools.

The clearest reproduction of the "thinking only, no final answer" failure mode was the long recursive listing case below.

## Findings

### 1. Missing final answer on a long listing task

- Case: `T11`
- Prompt: list `/home/sandbox` recursively up to 2 levels
- Result: `assistant.final` missing
- Stop reason: `max_tokens`
- Duration: `76.8 s`

The model kept iterating through file-search ideas and tool calls until it hit the token cap, then the turn ended without a final user-facing response.

Evidence:

- Trace: `/C:/Dev/Argus/.Argus/traces/966306c6-07b9-48c0-81db-96348bd1e903.jsonl`
- Summary: `scratch/aidebug-valid-runs/batch2/summary.json`

### 2. Copy prompt did not reach `server_copy`

- Case: `T32`
- Prompt: copy local `docs/README.md` to `sandbox-server:/tmp`
- Result: the model tried a local existence check, then asked for clarification instead of completing the copy
- Tool path observed: `glob`
- `server_copy` was not reached in this run

This is a tool-selection / file-resolution issue. The source file exists in the repo, but the agent stopped in the verification phase instead of completing the cross-server copy.

Evidence:

- Output: `scratch/aidebug-valid-runs/batch4/T32/stdout.txt`
- Summary: `scratch/aidebug-valid-runs/batch4/summary.json`

### 3. Slow outliers on large or chained operations

The average run time was reasonable, but a few prompts were materially slower:

- `T30` ownership / `touch + chmod + chown`: `107.5 s`
- `T11` recursive listing: `76.8 s`
- `T42` long recursive listing: `58.9 s`
- `T18` `journalctl -n 20 --no-pager`: `44.5 s`
- `T09` network listing: `33.5 s`

These are not correctness failures, but they are the main latency hotspots.

## UI Notes

- No native process crash was observed.
- No raw ANSI escape leakage was observed in the captured stdout files.
- Because the harness redirected stdout/stderr to files, the mirror rendered as plain text; that means full TUI color/motion fidelity was not exercised in this run.

## Tool Usage Pattern

Observed tool counts across the 50 tests:

- `bash`: 30
- `fs_list`: 9
- `server_inspect`: 6
- `server_metrics`: 4
- `fileread`: 2
- `glob`: 1
- `list_mcp_resources`: 1

## Notes

- `T31`, `T36`, `T45`, `T48`, and `T49` were handled as clarification-style responses without tool calls. Those prompts were underspecified enough that this was not automatically a bug.
- The count case was revalidated separately and completed successfully with a final answer.
- One stale artifact during analysis was revalidated directly before the report was finalized; the published batch summaries point at the validated run data.
