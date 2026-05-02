package query

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/services/workspace"
)

// laneSystemBlocks renders the live snapshot of every open execution lane so
// the LLM always sees the authoritative cwd / account stack and routes
// commands to the correct host even when several are connected at once.
func laneSystemBlocks(manager *workspace.Manager) []llm.SystemBlock {
	if manager == nil {
		return nil
	}
	infos := manager.LaneInfos()
	if len(infos) == 0 {
		return nil
	}

	sort.Slice(infos, func(i, j int) bool {
		if infos[i].Alias != infos[j].Alias {
			return infos[i].Alias < infos[j].Alias
		}
		return strings.Join(infos[i].AccountStack, ">") < strings.Join(infos[j].AccountStack, ">")
	})

	var sb strings.Builder
	sb.WriteString("ACTIVE EXECUTION LANES (single source of truth — use the `server` parameter to pick one):\n")
	now := time.Now()
	for _, info := range infos {
		user := topAccount(info.AccountStack)
		stack := strings.Join(info.AccountStack, ">")
		idle := formatIdle(now.Sub(info.LastUsed))
		fmt.Fprintf(&sb, "- %s : user=%s cwd=%s stack=%s idle=%s\n",
			info.Alias,
			defaultIfEmpty(user, "?"),
			defaultIfEmpty(info.CWD, "?"),
			defaultIfEmpty(stack, "-"),
			idle,
		)
	}
	sb.WriteString("\nRules (multi-channel routing is enforced):\n")
	sb.WriteString("- Each (alias, account_stack) pair runs on its own persistent SSH PTY channel.\n")
	sb.WriteString("  The router routes your command to the channel that matches the post-transition privilege\n")
	sb.WriteString("  automatically. You do not need to \"switch\" a channel by hand.\n")
	sb.WriteString("- To enter a privilege lane, send `sudo -i`, `sudo -i -u <user>`, or `su - <user>` as a\n")
	sb.WriteString("  normal bash command. A new channel is created and persisted; subsequent plain commands on\n")
	sb.WriteString("  that host will land on it until you send `exit`.\n")
	sb.WriteString("- To run a one-shot privileged command without staying in the lane, use\n")
	sb.WriteString("  `sudo -u <user> <body>`. The body runs on the current channel.\n")
	sb.WriteString("- Channels are isolated: changing cwd or env on one privilege lane does NOT affect another.\n")
	sb.WriteString("- The cwd / user shown above is authoritative for that channel.\n")
	sb.WriteString("- BEFORE sending any sudo/su command, check the elevation policy in the workspace block.\n")
	sb.WriteString("  If elevation is DISABLED for the target server, STOP and guide the user — do not call the tool.\n")

	return []llm.SystemBlock{{Type: "text", Text: sb.String()}}
}

func topAccount(stack []string) string {
	if len(stack) == 0 {
		return ""
	}
	return stack[len(stack)-1]
}

func defaultIfEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func formatIdle(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Second:
		return "0s"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}
