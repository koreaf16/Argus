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
	for _, entry := range entries {
		if entry.Kind == workspace.ServerKindLocal {
			label := "local (kind=local)"
			if entry.Alias == active {
				label += " [active]"
			}
			lines = append(lines, "- "+label)
			continue
		}

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
	}
	lines = append(lines, "When the user mentions one of these aliases (for example `oracle-server`), call the tool with that `server` alias directly instead of asking the user again.")
	lines = append(lines, "If the user explicitly says local machine / my PC / local workspace, set `server` to `local` on shell and file tools.")
	lines = append(lines, "Never include or request stored password values in your response.")

	blocks := []llm.SystemBlock{
		{Type: "text", Text: strings.Join(lines, "\n")},
	}

	if envBlock := buildActiveEnvBlock(manager, active); envBlock != "" {
		blocks = append(blocks, llm.SystemBlock{Type: "text", Text: envBlock})
	}

	return blocks
}

func buildActiveEnvBlock(manager *workspace.Manager, active string) string {
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

		if sh, err := utils.FindSuitableShell(); err == nil && sh != "" {
			fmt.Fprintf(&sb, "  shell: %s\n", sh)
		}

		if user := os.Getenv("USERNAME"); user != "" {
			fmt.Fprintf(&sb, "  user: %s\n", user)
		} else if user := os.Getenv("USER"); user != "" {
			fmt.Fprintf(&sb, "  user: %s\n", user)
		}

		sb.WriteString("\nBehavior rules:\n")
		if runtime.GOOS == "windows" {
			sb.WriteString("  - Active OS is Windows. You MUST use the 'powershell' tool for all system operations.\n")
			sb.WriteString("  - Do NOT attempt to use 'bash' as it is hidden and unavailable on this target.\n")
			sb.WriteString("  - Do NOT issue Linux-only commands (e.g. apt, yum, systemctl).\n")
		} else {
			sb.WriteString("  - Active OS is Unix-like. You MUST use the 'bash' tool for all system operations.\n")
			sb.WriteString("  - Do NOT attempt to use 'powershell' as it is hidden and unavailable on this target.\n")
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
		sb.WriteString("  - Shell tools execute on the REMOTE server by default (no `server` param needed).\n")
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
		sb.WriteString("  - Active OS is Windows. You MUST use the 'powershell' tool for all system operations.\n")
		sb.WriteString("  - Do NOT attempt to use 'bash' as it is hidden and unavailable on this target.\n")
	} else {
		sb.WriteString("  - Active OS is Unix-like. You MUST use the 'bash' tool for all system operations.\n")
		sb.WriteString("  - Do NOT attempt to use 'powershell' as it is hidden and unavailable on this target.\n")
	}
	sb.WriteString("  - Shell tools execute on the REMOTE server by default (no `server` param needed).\n")
	sb.WriteString("  - Use sudo/su for privilege escalation as appropriate for the remote OS.\n")

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
