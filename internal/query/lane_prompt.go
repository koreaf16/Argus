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
	sb.WriteString("활성 실행 레인 (ACTIVE EXECUTION LANES - 단일 진실 소스 — `server` 매개변수를 사용하여 선택):\n")
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
	sb.WriteString("\n규칙 (다중 채널 라우팅이 강제 적용됨):\n")
	sb.WriteString("- 각 (alias, account_stack) 쌍은 자체 영구 SSH PTY 채널에서 실행됩니다.\n")
	sb.WriteString("  라우터는 당신의 명령을 권한 전환 후의 권한과 일치하는 채널로 자동으로 라우팅합니다.\n")
	sb.WriteString("  채널을 수동으로 \"전환\"할 필요가 없습니다.\n")
	sb.WriteString("- 권한 레인에 진입하려면 일반 bash 명령으로 `sudo -i`, `sudo -i -u <user>` 또는 `su - <user>`를\n")
	sb.WriteString("  보내십시오. 새로운 채널이 생성되고 유지되며, 해당 호스트의 후속 일반 명령은 `exit`를\n")
	sb.WriteString("  보낼 때까지 해당 채널에 도달합니다.\n")
	sb.WriteString("- 레인에 머물지 않고 단발성 권한 명령을 실행하려면 `sudo -u <user> <body>`를 사용하십시오.\n")
	sb.WriteString("  본문(body)은 현재 채널에서 실행됩니다.\n")
	sb.WriteString("- 채널은 격리되어 있습니다: 한 권한 레인에서 cwd나 환경 변수를 변경해도 다른 레인에 영향을 주지 않습니다.\n")
	sb.WriteString("- 위에 표시된 cwd / user가 해당 채널에 대해 권위 있는 정보입니다.\n")
	sb.WriteString("- sudo/su 명령을 보내기 전에 반드시 워크스페이스 블록의 권한 상승 정책을 확인하십시오.\n")
	sb.WriteString("  대상 서버에 대해 권한 상승이 DISABLED인 경우, 중단하고 사용자를 안내하십시오 — 도구를 호출하지 마십시오.\n")

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
