package inventory

import (
	"fmt"
	"strings"
	"time"
)

const (
	maxContainersInPrompt = 10
	maxPodsInPrompt       = 10
	maxLLMModelsInPrompt  = 5
	tokenBudgetChars      = 6000 // ~1500 tokens at 4 chars/token
)

// FormatSummaryForPrompt renders a human+LLM readable summary of snap.
// Output is truncated to stay within tokenBudgetChars.
func FormatSummaryForPrompt(snap InventorySnapshot) string {
	if snap.Status == StatusPending {
		return "서버 인벤토리: 백그라운드 수집 진행 중. 다음 턴에서 자동 갱신됩니다."
	}

	loc, _ := time.LoadLocation("Asia/Seoul")
	ts := snap.CollectedAt.In(loc).Format("2006-01-02 15:04 KST")

	var sb strings.Builder
	fmt.Fprintf(&sb, "서버 인벤토리 (alias=%s, 수집=%s, %.1fs):\n\n",
		snap.Alias, ts, float64(snap.DurationMs)/1000)

	if snap.System != nil {
		s := snap.System
		os := s.OSPretty
		if os == "" {
			os = s.OS
		}
		if os != "" {
			fmt.Fprintf(&sb, "[시스템] OS: %s", os)
			if s.User != "" {
				fmt.Fprintf(&sb, ", user: %s", s.User)
			}
			if s.Shell != "" {
				fmt.Fprintf(&sb, ", shell: %s", s.Shell)
			}
			if s.Uptime != "" {
				fmt.Fprintf(&sb, ", uptime: %s", s.Uptime)
			}
			if s.ServiceCount > 0 || s.ListenerCount > 0 {
				fmt.Fprintf(&sb, ", services: %d running / %d listeners", s.ServiceCount, s.ListenerCount)
			}
			sb.WriteString("\n\n")
		}
	}

	if len(snap.Containers) > 0 {
		running := 0
		for _, c := range snap.Containers {
			if c.State == "running" {
				running++
			}
		}
		fmt.Fprintf(&sb, "[컨테이너] docker: %d개 (running %d / exited %d)\n",
			len(snap.Containers), running, len(snap.Containers)-running)
		shown := snap.Containers
		if len(shown) > maxContainersInPrompt {
			shown = shown[:maxContainersInPrompt]
		}
		for _, c := range shown {
			fmt.Fprintf(&sb, "  - %s (%s) %s [%s]\n", c.Name, c.Image, c.Ports, c.State)
		}
		if len(snap.Containers) > maxContainersInPrompt {
			fmt.Fprintf(&sb, "  ...외 %d개\n", len(snap.Containers)-maxContainersInPrompt)
		}
		sb.WriteString("\n")
	}

	if snap.Kubernetes != nil {
		k := snap.Kubernetes
		nsStr := ""
		if len(k.Namespaces) > 0 {
			shown := k.Namespaces
			if len(shown) > 6 {
				shown = shown[:6]
			}
			nsStr = strings.Join(shown, ", ")
			if len(k.Namespaces) > 6 {
				nsStr += fmt.Sprintf(" 외 %d개", len(k.Namespaces)-6)
			}
		}
		fmt.Fprintf(&sb, "[Kubernetes] context=%s, perm=%s, nodes=%d, pods=%d\n",
			k.Context, k.Permission, k.NodeCount, k.PodCount)
		if nsStr != "" {
			fmt.Fprintf(&sb, "  네임스페이스: %s\n", nsStr)
		}
		if len(k.Workloads) > 0 {
			sb.WriteString("  주요 워크로드:\n")
			shown := k.Workloads
			if len(shown) > maxPodsInPrompt {
				shown = shown[:maxPodsInPrompt]
			}
			for _, w := range shown {
				fmt.Fprintf(&sb, "    - %s/%s  (%s, image=%s)\n",
					w.Namespace, w.Name, w.Phase, w.Image)
			}
			if len(k.Workloads) > maxPodsInPrompt {
				fmt.Fprintf(&sb, "    ...외 %d개\n", len(k.Workloads)-maxPodsInPrompt)
			}
		}
		sb.WriteString("\n")
	}

	if len(snap.LLMServing) > 0 {
		sb.WriteString("[LLM Serving] (모델명 질의 라우팅용)\n")
		for _, llm := range snap.LLMServing {
			hostStr := formatHosting(llm.Hosting)
			if len(llm.Models) == 0 {
				fmt.Fprintf(&sb, "  - %-8s @ :%d  hosting=%s\n", llm.Engine, llm.Port, hostStr)
				continue
			}
			shown := llm.Models
			if len(shown) > maxLLMModelsInPrompt {
				shown = shown[:maxLLMModelsInPrompt]
			}
			for i, m := range shown {
				if i == 0 {
					fmt.Fprintf(&sb, "  - %-8s → %-40s @ :%d  hosting=%s\n",
						llm.Engine, m.ID, llm.Port, hostStr)
				} else {
					fmt.Fprintf(&sb, "             → %-40s\n", m.ID)
				}
			}
			if len(llm.Models) > maxLLMModelsInPrompt {
				fmt.Fprintf(&sb, "             ...외 %d개\n", len(llm.Models)-maxLLMModelsInPrompt)
			}
		}
		sb.WriteString("\n")
	}

	statusNote := ""
	switch snap.Status {
	case StatusReady:
		statusNote = "상태: ready (모든 probe 성공)"
	case StatusPartial:
		statusNote = fmt.Sprintf("상태: partial (%d개 probe 실패)", len(snap.Errors))
	case StatusFailed:
		statusNote = "상태: failed"
	}
	fmt.Fprintf(&sb, "%s\n주의: 5분간 유효. 실시간 확인은 server_inventory 도구를 사용하세요.\n", statusNote)

	result := sb.String()
	if len(result) > tokenBudgetChars {
		result = result[:tokenBudgetChars] + "\n...(truncated)\n"
	}
	return result
}

// FormatUICard returns compact display lines for the TUI connect card.
// Format: "category   value" (3 spaces) for category lines, "   value" for continuations.
// System lines come first, followed by docker/k8s/llm lines.
func FormatUICard(snap InventorySnapshot) []string {
	var lines []string

	if snap.System != nil {
		s := snap.System
		osLabel := s.OSPretty
		if osLabel == "" {
			osLabel = s.OS
		}
		if osLabel != "" {
			lines = append(lines, "os   "+osLabel)
		}
		userLabel := s.User
		if s.Shell != "" {
			shell := s.Shell
			if idx := strings.LastIndex(shell, "/"); idx >= 0 {
				shell = shell[idx+1:]
			}
			if userLabel != "" {
				userLabel += " / " + shell
			} else {
				userLabel = shell
			}
		}
		if userLabel != "" {
			lines = append(lines, "user   "+userLabel)
		}
		if s.Uptime != "" {
			lines = append(lines, "uptime   "+s.Uptime)
		}
		if s.ServiceCount > 0 || s.ListenerCount > 0 {
			lines = append(lines, fmt.Sprintf("services   %d running / %d listeners", s.ServiceCount, s.ListenerCount))
		}
	}

	if len(snap.Containers) > 0 {
		running := 0
		for _, c := range snap.Containers {
			if c.State == "running" {
				running++
			}
		}
		lines = append(lines, fmt.Sprintf("docker   %d containers (%d running)", len(snap.Containers), running))
	}
	if snap.Kubernetes != nil {
		k := snap.Kubernetes
		lines = append(lines, fmt.Sprintf("k8s      %d nodes / %d pods (%s)", k.NodeCount, k.PodCount, k.Permission))
	}
	for _, llm := range snap.LLMServing {
		hostStr := formatHosting(llm.Hosting)
		if len(llm.Models) == 0 {
			lines = append(lines, fmt.Sprintf("%-9s@ :%d  →  %s", llm.Engine, llm.Port, hostStr))
			continue
		}
		for i, m := range llm.Models {
			if i == 0 {
				lines = append(lines, fmt.Sprintf("%-9s%-36s @ :%d  →  %s", llm.Engine, m.ID, llm.Port, hostStr))
			} else {
				lines = append(lines, fmt.Sprintf("         %-36s", m.ID))
			}
		}
	}
	return lines
}

func formatHosting(h LLMHosting) string {
	switch h.Type {
	case "k8s":
		if h.K8sPod != "" {
			return "k8s pod (" + h.K8sPod + ")"
		}
		return "k8s pod"
	case "docker":
		if h.Container != "" {
			return "docker (" + h.Container + ")"
		}
		return "docker"
	case "systemd":
		return "systemd"
	case "native":
		return "native"
	default:
		return "unknown"
	}
}
