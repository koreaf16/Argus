package query

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/services/workspace"
	"github.com/koreaf16/argus/internal/utils"
)

func workspaceSystemBlocks(manager *workspace.Manager) []llm.SystemBlock {
	if manager == nil || manager.Registry() == nil {
		return nil
	}
	entries := manager.Registry().List()
	if len(entries) == 0 {
		return nil
	}

	active := manager.ActiveAlias()
	lines := []string{"Available workspaces (registered aliases):"}
	remoteCount := 0
	hasDisabledElevation := false
	for _, entry := range entries {
		if entry.Kind == workspace.ServerKindLocal {
			label := "local (kind=local)"
			if entry.Alias == active {
				label += " [active]"
			}
			lines = append(lines, "- "+label)
			continue
		}

		remoteCount++
		desc := fmt.Sprintf("%s (kind=%s, user=%s, host=%s, port=%d)",
			entry.Alias,
			entry.Kind,
			strings.TrimSpace(entry.User),
			strings.TrimSpace(entry.Host),
			entry.Port,
		)
		if entry.Alias == active {
			desc += " [active]"
		}
		lines = append(lines, "- "+desc)
		// Elevation status line
		e := entry.Elevation
		if e.Allowed && e.Mode != "none" {
			targets := strings.Join(e.TargetUsers, ", ")
			if targets == "" {
				targets = "any"
			}
			lines = append(lines, fmt.Sprintf("               elevation: ENABLED   mode=%s   targets=%s", e.Mode, targets))
		} else {
			lines = append(lines, fmt.Sprintf("               elevation: DISABLED  (sudo/su will be refused)"))
			hasDisabledElevation = true
		}
	}
	lines = append(lines, "CONNECTION INTENT: When the user asks to connect to, switch to, or access a server (e.g. '접속해', '접속', 'connect to X', 'switch to X', 'X로 바꿔', 'X 워크스페이스로'), you MUST call the `server_connect` tool with `server`=alias. NEVER use bash/powershell with an ssh command for this purpose.")
	lines = append(lines, "If the alias is registered (listed above) but not yet connected, call `server_connect` with just the `server` field.")
	lines = append(lines, "If the alias is NOT registered but the user provides host/user info, call `server_connect` with `server`, `host`, and `user` fields to auto-register and connect.")
	lines = append(lines, "When the user mentions one of these aliases (for example `oracle-server`), call the tool with that `server` alias directly instead of asking the user again.")
	lines = append(lines, "If the user mentions a server alias that does NOT appear in the list above, do NOT call server_connect or any other tool. Instead, immediately tell the user (in Korean) that the server is not registered and list the available server aliases.")
	lines = append(lines, "If the user explicitly says local machine / my PC / local workspace, set `server` to `local` on shell and file tools.")
	if remoteCount >= 2 {
		lines = append(lines, "MULTI-WORKSPACE SAFETY: two or more remote workspaces are registered. You MUST explicitly set `server` on shell/file/search/inspect/metrics/account-shell tools; calls without `server` will be rejected.")
		lines = append(lines, "If an active workflow defines a role profile, you may set `role`/`channel` instead; the role must resolve to exactly one server and mismatched server+role calls will be rejected.")
		lines = append(lines, "For `server_copy`, you MUST explicitly provide both source and destination aliases (either alias:path on both sides or both `src_server` and `dst_server`).")
	}
	lines = append(lines, "Never include or request stored password values in your response.")

	// Elevation policy section (only added when there are remote servers)
	if remoteCount > 0 {
		lines = append(lines, elevationPolicyPrompt(hasDisabledElevation))
	}

	blocks := []llm.SystemBlock{
		{Type: "text", Text: strings.Join(lines, "\n")},
	}

	if envBlock := buildActiveEnvBlock(manager, active, remoteCount >= 2); envBlock != "" {
		blocks = append(blocks, llm.SystemBlock{Type: "text", Text: envBlock})
	}

	return blocks
}

func buildActiveEnvBlock(manager *workspace.Manager, active string, strictMultiWorkspace bool) string {
	entry, ok := manager.Registry().Get(active)
	if !ok {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Active environment:\n")
	fmt.Fprintf(&sb, "  workspace: %s\n", active)

	if entry.Kind == workspace.ServerKindLocal {
		fmt.Fprintf(&sb, "  kind: local\n")
		fmt.Fprintf(&sb, "  os: %s/%s\n", runtime.GOOS, runtime.GOARCH)

		if runtime.GOOS == "windows" {
			fmt.Fprintf(&sb, "  preferred shell tool: powershell\n")
			if sh, err := utils.FindSuitableShell(); err == nil && sh != "" {
				fmt.Fprintf(&sb, "  bash compatibility shell: %s\n", sh)
			}
		} else if sh, err := utils.FindSuitableShell(); err == nil && sh != "" {
			fmt.Fprintf(&sb, "  preferred shell tool: bash\n")
			fmt.Fprintf(&sb, "  shell: %s\n", sh)
		}

		if user := os.Getenv("USERNAME"); user != "" {
			fmt.Fprintf(&sb, "  user: %s\n", user)
		} else if user := os.Getenv("USER"); user != "" {
			fmt.Fprintf(&sb, "  user: %s\n", user)
		}

		sb.WriteString("\nBehavior rules:\n")
		if runtime.GOOS == "windows" {
			sb.WriteString("  - Active OS is Windows. Use the 'powershell' tool for LOCAL system operations.\n")
			sb.WriteString("  - Do NOT use 'bash' for local Windows commands. Use 'bash' only when the tool call explicitly targets a Unix-like remote workspace via `server` or `role`.\n")
			sb.WriteString("  - Do NOT issue Linux-only commands (e.g. apt, yum, systemctl).\n")
		} else {
			sb.WriteString("  - Active OS is Unix-like. Use the 'bash' tool for LOCAL system operations.\n")
			sb.WriteString("  - Do NOT use 'powershell' for local Unix-like commands. Use 'powershell' only when the tool call explicitly targets a Windows workspace via `server` or `role`.\n")
		}
		sb.WriteString("  - Shell tools execute on the LOCAL machine unless you specify a `server` parameter.\n")
		return sb.String()
	}

	// Remote (SSH)
	fmt.Fprintf(&sb, "  kind: ssh\n")
	fmt.Fprintf(&sb, "  host: %s@%s:%d\n",
		strings.TrimSpace(entry.User),
		strings.TrimSpace(entry.Host),
		entry.Port,
	)

	snap, hasSnap := manager.GetInspectSnapshot(active)
	if !hasSnap {
		sb.WriteString("  environment: not yet inspected — run server_inspect to gather OS/service/port info\n")
		sb.WriteString("\nBehavior rules:\n")
		if strictMultiWorkspace {
			sb.WriteString("  - Multiple remote workspaces are registered. You MUST set `server` explicitly on each shell/file/search/inspect/metrics call.\n")
		} else {
			sb.WriteString("  - Shell tools execute on the REMOTE server by default (no `server` param needed).\n")
		}
		sb.WriteString("  - To run commands on the LOCAL machine, specify server=\"local\" in the tool call.\n")
		return sb.String()
	}

	if snap.OS != "" {
		fmt.Fprintf(&sb, "  os: %s\n", firstLine(snap.OS))
	}
	if snap.Shell != "" {
		fmt.Fprintf(&sb, "  shell: %s\n", firstLine(snap.Shell))
	}
	if snap.User != "" {
		fmt.Fprintf(&sb, "  user: %s\n", snap.User)
	}
	if snap.CWD != "" {
		fmt.Fprintf(&sb, "  cwd: %s\n", snap.CWD)
	}
	if snap.Uptime != "" {
		fmt.Fprintf(&sb, "  uptime: %s\n", snap.Uptime)
	}
	if snap.Memory != "" {
		fmt.Fprintf(&sb, "  memory:\n%s\n", indent(snap.Memory, "    "))
	}
	if snap.Listeners != "" {
		fmt.Fprintf(&sb, "  listening ports:\n%s\n", indent(truncateLines(snap.Listeners, 15), "    "))
	}
	if snap.Services != "" {
		fmt.Fprintf(&sb, "  running services:\n%s\n", indent(truncateLines(snap.Services, 20), "    "))
	}
	if snap.Docker != "" {
		fmt.Fprintf(&sb, "  docker containers:\n%s\n", indent(snap.Docker, "    "))
	}

	sb.WriteString("\nBehavior rules:\n")
	if strings.Contains(strings.ToLower(snap.OS), "windows") {
		sb.WriteString("  - Active OS is Windows. Use the 'powershell' tool for this workspace.\n")
		sb.WriteString("  - Do NOT use 'bash' for this Windows workspace. Use 'bash' only when explicitly targeting a different Unix-like workspace.\n")
	} else {
		sb.WriteString("  - Active OS is Unix-like. Use the 'bash' tool for this workspace.\n")
		sb.WriteString("  - Do NOT use 'powershell' for this Unix-like workspace. Use 'powershell' only when explicitly targeting a different Windows workspace.\n")
	}
	if strictMultiWorkspace {
		sb.WriteString("  - Multiple remote workspaces are registered. You MUST set `server` explicitly on each shell/file/search/inspect/metrics call.\n")
	} else {
		sb.WriteString("  - Shell tools execute on the REMOTE server by default (no `server` param needed).\n")
	}
	sb.WriteString("  - To run commands on the LOCAL machine, specify server=\"local\" in the tool call.\n")
	sb.WriteString("  - Check the 'elevation' line in the workspace list before ANY sudo/su command.\n")
	sb.WriteString("    If elevation is DISABLED for this server, STOP and guide the user to /server edit <alias>.\n")

	return sb.String()
}

// elevationPolicyPrompt returns the LLM instruction block that explains how
// to interpret the per-server elevation lines and what to do before calling
// bash with a sudo/su command.
func elevationPolicyPrompt(anyDisabled bool) string {
	sb := &strings.Builder{}
	sb.WriteString("\nELEVATION POLICY (per-server, enforced — STOP AND ASK BEFORE TRYING):\n")
	sb.WriteString("Each server has its own elevation policy. Before running ANY sudo/su\n")
	sb.WriteString("command, look at the alias's elevation line above and decide:\n\n")

	sb.WriteString("  Case 1) elevation: DISABLED\n")
	sb.WriteString("    ► STOP. Do NOT call any tool. Do NOT attempt the sudo command even\n")
	sb.WriteString("      \"to see what happens\". Respond to the user in Korean with:\n")
	sb.WriteString("        - which command needed root (quote the command)\n")
	sb.WriteString("        - why you stopped (the server's elevation policy is OFF)\n")
	sb.WriteString("        - how to enable it: \"`/server edit <alias>` 명령으로 elevation을\n")
	sb.WriteString("          켜고 sudo 비밀번호를 등록하신 뒤 다시 요청해 주세요\"\n")
	sb.WriteString("      End the turn. Do not retry until the user re-requests after\n")
	sb.WriteString("      registering the policy.\n\n")

	sb.WriteString("  Case 2) elevation: ENABLED, target user is in targets (or targets=any)\n")
	sb.WriteString("    ► Proceed normally. The channel injects the sudo password from the\n")
	sb.WriteString("      credential store (mode=password) or SSH login password\n")
	sb.WriteString("      (mode=reuse_login). Do not ask the user for the password.\n\n")

	sb.WriteString("  Case 3) elevation: ENABLED but the requested target user is NOT in targets\n")
	sb.WriteString("    ► STOP. Tell the user (in Korean) which target user was requested\n")
	sb.WriteString("      and that it is outside the allow-list. Suggest /server edit <alias>.\n")
	sb.WriteString("      Do not call the tool.\n\n")

	sb.WriteString("- NEVER ask the user for a sudo/su password directly. Passwords are\n")
	sb.WriteString("  registered through `/server edit <alias>`.\n")
	sb.WriteString("- The channel router has its own reject layer as a safety net. If you\n")
	sb.WriteString("  hit it, treat it as a bug in your reasoning — you should have stopped\n")
	sb.WriteString("  earlier based on the policy above.")

	if anyDisabled {
		sb.WriteString("\n\nIMPORTANT: One or more servers have elevation DISABLED. For those\n")
		sb.WriteString("servers you MUST stop and guide the user to /server edit <alias> before any\n")
		sb.WriteString("sudo/su attempt. This is a hard stop, not a suggestion.\n")
		sb.WriteString("MIGRATION NOTE: If this is an existing setup that used sudo without\n")
		sb.WriteString("an explicit policy, the user can set ARGUS_ELEVATION_LEGACY=1 in their\n")
		sb.WriteString("environment as a temporary fallback (reuse_login for all servers) while\n")
		sb.WriteString("they register explicit policies via /server edit <alias>.")
	}

	return sb.String()
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return strings.TrimSpace(s)
}

func indent(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}

func truncateLines(s string, max int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= max {
		return s
	}
	return strings.Join(lines[:max], "\n") + fmt.Sprintf("\n... (%d more lines)", len(lines)-max)
}
