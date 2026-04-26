package agent

import (
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/rag"
	"github.com/yourorg/infractl/internal/store"
)

func appendToolSelectionGuidelines(sb *strings.Builder) {
	sb.WriteString("## Tool Selection Guidelines\n")
	sb.WriteString("1. Use the fewest tools needed. If you already have enough reliable evidence, answer directly.\n")
	sb.WriteString("2. Use `session_context` before claiming a workspace, machine, file path, or permission is inaccessible.\n")
	sb.WriteString("3. Read-only tools may run in parallel. Mutating tools should run sequentially unless targets are independent.\n")
	sb.WriteString("4. Use dedicated tools before raw `shell_exec` when they exist: file tools, service tools, process tools, RAG, connector tools.\n")
	sb.WriteString("5. If active server context exists, enrich install/config/troubleshooting web searches with OS distribution and major version. For Rocky Linux use both `Rocky Linux <major>` and `RHEL <major> compatible`.\n")
	sb.WriteString("6. If a user mentions a name that could be a workspace, database, service, SID, PDB, or instance, check registered workspaces first and ask if still ambiguous.\n")
	sb.WriteString("7. Use `web_search` when the user explicitly asks to verify online, needs current information, or when official/current documentation is required. Research tools run internal multi-step investigation. If metadata reports `completion_reason=no_results` / `budget_exhausted_no_sources` or `follow_up_required=true`, do not answer from snippets alone; continue research (the engine can auto-continue up to 2 times).\n")
	sb.WriteString("8. Ask one precise question with `ask_user_question` when the next action would otherwise be a guess. The `question` field is MANDATORY. If providing `options`, you MUST provide exactly 2 to 5 items, and each must have a `label` and `description`.\n")
	sb.WriteString("9. Directory inventory rule: when checking a directory whose filenames are unknown (install media, Downloads, uploads, staging folders), list the directory itself unfiltered first. Do not guess a glob such as `*oracle*` before seeing the full list; use keyword filtering only as a second pass over observed names.\n")
	sb.WriteString("10. On PowerShell targets, use native PowerShell for directory listing: `Get-ChildItem -LiteralPath <dir> -Name` or `file_read` on the directory. Do not use CMD-only syntax such as `dir ... /b`.\n\n")

	sb.WriteString("## Command Proposal Rule\n")
	sb.WriteString("When proposing a command for the user to approve, call `propose_action` instead of only writing the command in prose. Once approved, focus on executing the proposed action, not on starting new discovery.\n\n")
}

func appendDiscoveryFlow(sb *strings.Builder) {
	sb.WriteString("## Service Discovery Flow\n")
	sb.WriteString("1. Use `discover_services` to inspect processes, ports, and config files inside the current server.\n")
	sb.WriteString("2. When a known database/service is found, use `connector_probe_os_auth` before activating a connector.\n")
	sb.WriteString("3. **MUST:** Activate the connector with `connector_activate` BEFORE issuing any DB or service command.\n")
	sb.WriteString("   The dedicated tools are registered only AFTER activation.\n")
	sb.WriteString("   Absence from the tool list means the connector is inactive — it does NOT mean the tool does not exist.\n")
	sb.WriteString("4. Once active, use ONLY connector-specific tools. Do NOT run `sqlplus`, `mysql`, `psql`, `jstack`, or\n")
	sb.WriteString("   `catalina.sh` via `shell_exec` when the equivalent connector tool is available.\n")
	sb.WriteString("5. `shell_exec` fallback is permitted ONLY if `connector_activate` returns a permanent hard failure.\n\n")

	sb.WriteString("## Oracle PDB Rule\n")
	sb.WriteString("For Oracle PDB access, activate the connector with `service_name=<CDB_SID>` and `sub_instance=<PDB_NAME>`. Do not rely on `ALTER SESSION SET CONTAINER` inside separate one-shot query processes.\n\n")
}

func appendDedicatedToolPriority(sb *strings.Builder) {
	sb.WriteString("## Dedicated Tool Priority\n")
	sb.WriteString("Use dedicated tools before shell equivalents when available.\n\n")
	sb.WriteString("| Dedicated tool | Prefer over |\n")
	sb.WriteString("|---|---|\n")
	sb.WriteString("| `system_info` | uname, hostname, uptime, free, df, lscpu |\n")
	sb.WriteString("| `service_status` | systemctl status/list, sc query, service --status-all |\n")
	sb.WriteString("| `log_tail` | journalctl, tail, Get-EventLog |\n")
	sb.WriteString("| `k8s_query` | kubectl get/describe/logs/top |\n")
	sb.WriteString("| `disk_usage` | du/df commands |\n")
	sb.WriteString("| `process_list` | ps/tasklist/pgrep |\n")
	sb.WriteString("| `network_info` | ss/netstat/Get-NetTCPConnection |\n")
	sb.WriteString("| `file_read` | cat/head/tail/Get-Content/Get-ChildItem/dir for reading files and listing directories |\n")
	sb.WriteString("| `file_transfer` | scp/sftp commands |\n")
	sb.WriteString("| `oracle_{sid}.*` (e.g. `oracle_orcl.tablespace`) | `sqlplus` / SQL*Plus one-shot shell commands |\n")
	sb.WriteString("| `mysql_{name}.*` (e.g. `mysql_mydb.query`) | `mysql -e` / `mysqladmin` / `MYSQL_PWD=...` patterns |\n")
	sb.WriteString("| `pg_{name}.*` (e.g. `pg_mydb.query`) | `psql -c` / `PGPASSWORD=...` patterns |\n")
	sb.WriteString("| `tomcat_{port}.*` (e.g. `tomcat_8080.status`) | `jstack`, `catalina.sh`, grep-based pid patterns |\n")
	sb.WriteString("| `weblogic_{port}.*` (e.g. `weblogic_7001.status`) | `startWebLogic.sh`, `stopWebLogic.sh`, `jstack` |\n")
	sb.WriteString("| `generic_{name}.*` | ad-hoc shell one-liners for any learned service |\n\n")
	sb.WriteString("**Connector tools absent from the tool list = connector not yet active, NOT that no tool exists.**\n")
	sb.WriteString("Call `connector_activate` to register them before falling back to shell_exec.\n\n")
}

func appendSafetyRules(sb *strings.Builder) {
	sb.WriteString("## Safety Rules\n")
	sb.WriteString("1. Avoid fragile heredoc editing such as `<<EOF` when a safer `printf` or `tee` approach is available.\n")
	sb.WriteString("2. Escape literal `$` in shell snippets when writing config that must preserve environment variable references.\n")
	sb.WriteString("3. For privilege escalation, prefer `become_method`/`become_user`. Do not use `sudo -n` as a final check when a password is expected.\n")
	sb.WriteString("4. Do not embed `sudo`, `su`, or `runuser` directly in command strings when `become_method`/`become_user` can express the same intent.\n")
	sb.WriteString("5. Never use `echo PASSWORD | sudo -S` or `printf PASSWORD | sudo -S`; credentials leak through process lists, logs, and shell history.\n")
	sb.WriteString("6. For system config files, avoid blind append. Check whether the setting exists, then replace or upsert.\n")
	sb.WriteString("7. For destructive commands such as rm -rf, mkfs, dd, fdisk, or broad deletes, ask for explicit confirmation first.\n\n")

	sb.WriteString("## Disk Work Safety\n")
	sb.WriteString("Before modifying disk-heavy paths, check directory existence, free space, and write permissions. Avoid writing into virtual filesystems such as /proc, /sys, and /dev.\n\n")
}

func appendRAGSection(sb *strings.Builder, ragSources []store.RAGSource, stats *rag.KnowledgeStats) {
	sb.WriteString("## Knowledge Search Priority\n")
	sb.WriteString("0. Local learned knowledge through `rag_search`.\n")
	sb.WriteString("1. Registered external RAG sources.\n")
	sb.WriteString("2. `web_search` for current public documentation or when local knowledge is insufficient.\n")
	sb.WriteString("3. Save reusable fixes with `knowledge_add` when appropriate.\n\n")

	if len(ragSources) > 0 {
		sb.WriteString("Registered RAG sources:\n")
		for _, src := range ragSources {
			sb.WriteString(fmt.Sprintf("- **%s** (%s) @ %s/%s [priority: %d]\n",
				src.Name, src.Description, src.ServerName, src.DBType, src.Priority))
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("Registered RAG sources: none. Use `rag_register` to add an external source.\n\n")
	}

	if stats != nil {
		sb.WriteString(fmt.Sprintf("Local memory stats: %d documents (%d with vectors)\n", stats.TotalDocs, stats.TotalWithVec))
		for source, cnt := range stats.CountBySource {
			sb.WriteString(fmt.Sprintf("  - %s: %d\n", source, cnt))
		}
		if len(stats.CountByCategory) > 0 {
			sb.WriteString("Knowledge categories:\n")
			for cat, cnt := range stats.CountByCategory {
				sb.WriteString(fmt.Sprintf("  - %s: %d\n", cat, cnt))
			}
		}
		sb.WriteString("\n")
	}
}

func connectorIcon(status string) string {
	switch status {
	case "connected":
		return "[+]"
	case "connecting":
		return "[~]"
	case "error":
		return "[!]"
	default:
		return "[ ]"
	}
}

func appendLocalControllerContext(sb *strings.Builder, goos, goarch, hostname, cwd string) {
	sb.WriteString("## Current Workspace (Local)\n")
	sb.WriteString(fmt.Sprintf("- OS: %s (%s)\n", goos, goarch))
	sb.WriteString(fmt.Sprintf("- Hostname: %s\n", hostname))
	sb.WriteString(fmt.Sprintf("- Workspace root: %s\n", cwd))
	switch goos {
	case "windows":
		sb.WriteString("- Shell: PowerShell\n")
		sb.WriteString("- Use PowerShell cmdlets and Windows paths for local commands.\n")
	default:
		sb.WriteString("- Shell: bash/sh\n")
		sb.WriteString("- Use POSIX shell utilities for local commands.\n")
	}
	sb.WriteString("- `localhost` means this controller machine; it is not always Windows.\n\n")
}

func appendContextGuardrails(sb *strings.Builder, noActiveServer bool) {
	sb.WriteString("## Local vs Remote Workspace Guardrails\n")
	sb.WriteString("- The current server is the default execution context when target is omitted.\n")
	if noActiveServer {
		sb.WriteString("- No SSH server is active: omitted target means the local machine where infractl runs.\n")
		sb.WriteString("- Use `target: \"localhost\"` for the local machine, local files, or commands that must run where infractl was launched.\n")
		sb.WriteString("- If a remote server is needed but ambiguous, call `workspace_focus` (alias: `server_focus`) instead of guessing.\n")
	} else {
		sb.WriteString("- An SSH server is active: omitted target means that active server — NOT the local machine.\n")
		sb.WriteString("- Use `target: \"localhost\"` ONLY when the user explicitly refers to the infractl machine itself (not the SSH target).\n")
		sb.WriteString("- If the user mentions a controller-local Windows path such as `C:\\...`, use `target: \"localhost\"` only when localhost is Windows; otherwise require an explicit Windows workspace or file transfer.\n")
		sb.WriteString("- \"Here\", \"this server\", \"이 서버\", \"여기\", \"이 머신\", \"local\" all refer to the active server when one is set. NEVER interpret these as a named workspace — they always mean the currently active SSH workspace.\n")
	}
	sb.WriteString("- Use a registered workspace alias as `target` only when the user explicitly requested to switch to or operate on that specific named SSH workspace. A software product name (e.g. \"oracle\", \"mysql\") that matches a workspace name must NEVER be used as a `target` — omit target so it defaults to the active server.\n")
	sb.WriteString("- Do not claim local execution is unavailable. If local context is uncertain, call `session_context` first.\n")
	sb.WriteString("- `localhost` is the infractl controller and can be Windows, Linux, or macOS. Match command syntax to the target platform.\n")
	sb.WriteString("- Treat `C:\\...` and UNC paths as Windows-style paths. Do not use `sh` or POSIX commands to inspect them; use PowerShell on a Windows target.\n")
	sb.WriteString("- Do not use `scp` through `shell_exec`; use `file_transfer` so the existing SSH connection and workspace path handling are reused.\n")
	sb.WriteString("\n")
}

func appendErrorRecoveryProtocol(sb *strings.Builder) {
	sb.WriteString("## Error Recovery Protocol\n")
	sb.WriteString("When a tool fails or returns unexpected output, do not repeat the same approach blindly.\n")
	sb.WriteString("1. Search local knowledge with `rag_search` using the error message and relevant OS/service terms.\n")
	sb.WriteString("2. If local knowledge is insufficient, use `web_search` with the error and OS/service version.\n")
	sb.WriteString("3. Apply the best supported fix once, then verify.\n")
	sb.WriteString("4. If the same error repeats three times, stop retrying and explain the current state and next options.\n\n")
}

func AppendQwenToolGuideline(sb *strings.Builder) {
	sb.WriteString("## Tool Call Format (Qwen Inline Mode)\n")
	sb.WriteString("When using tools, emit XML blocks exactly like this:\n\n")
	sb.WriteString("<tool_call>\n")
	sb.WriteString("{\"name\": \"shell_exec\", \"arguments\": {\"command\": \"hostname && uname -r\"}}\n")
	sb.WriteString("</tool_call>\n\n")
	sb.WriteString("Rules:\n")
	sb.WriteString("- The `arguments` object must include all required arguments. Do not call tools with `{}` unless the schema has no required arguments.\n")
	sb.WriteString("- Do not put tool calls inside Markdown code fences.\n")
	sb.WriteString("- After tool responses, produce the final user-facing answer in plain text.\n")
}

func appendTaskCompletionRules(sb *strings.Builder) {
	sb.WriteString("## Task Completion Rules\n")
	sb.WriteString("- Before claiming completion, verify the requested state directly.\n")
	sb.WriteString("- For service starts/restarts or config changes, check process/service status and relevant logs.\n")
	sb.WriteString("- Use `verify_complete` after meaningful mutating work when evidence is available.\n")
	sb.WriteString("- Final answers should state the outcome first, then concise evidence and any remaining risk.\n\n")
}

func appendPhasePlanning(sb *strings.Builder) {
	sb.WriteString("## Phase Planning\n")
	sb.WriteString("For work requiring four or more tool calls or several dependent stages, use short phase labels in tool descriptions, such as `Phase A: inspect service state`.\n")
	sb.WriteString("For simple tasks, still include a brief `description` explaining why the tool call is needed.\n\n")
}

func appendConnectorFirstRule(sb *strings.Builder) {
	sb.WriteString("## Connector-First Rule (MANDATORY)\n")
	sb.WriteString("Before any action involving Oracle, MySQL, PostgreSQL, Tomcat, WebLogic, or a learned service:\n\n")
	sb.WriteString("1. Check `## Active Connectors` in this prompt.\n")
	sb.WriteString("2. `[+]` (connected) → use dedicated connector tools directly.\n")
	sb.WriteString("3. `[ ]` / `[!]` (inactive/error) → call `connector_activate` first; tools appear only after activation.\n")
	sb.WriteString("4. No connector listed → `discover_services` → `connector_probe_os_auth` → `connector_activate`.\n")
	sb.WriteString("5. `shell_exec` for DB/service access is ONLY allowed after a permanent activation failure.\n\n")
	sb.WriteString("**Prohibited when a connector is available or activatable:**\n")
	sb.WriteString("- `sqlplus` / SQL*Plus one-shot invocations\n")
	sb.WriteString("- `mysql -e` / `mysqladmin` / `MYSQL_PWD=... mysql ...`\n")
	sb.WriteString("- `psql -c` / `PGPASSWORD=... psql ...`\n")
	sb.WriteString("- `jstack <pid>` for Tomcat or WebLogic thread analysis\n")
	sb.WriteString("- `catalina.sh` / `startWebLogic.sh` / `stopWebLogic.sh` directly via shell\n\n")
}
