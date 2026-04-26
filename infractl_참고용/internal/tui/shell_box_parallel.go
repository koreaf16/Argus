// Package tui
// File: shell_box_parallel.go
// Description: 병렬 서브태스크 기반 도구(discover_services, system_info, network_info)의 박스 렌더러
// Responsibility: 각 서브태스크의 실시간 상태(running/done/fail/skip)를 박스로 통합 표시

package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// renderParallelTaskBox는 서브태스크가 있는 도구의 상태 박스를 렌더링한다.
func renderParallelTaskBox(name string, args map[string]any, lines []string, icon boxIcon, dur time.Duration, width int) string {
	boxW := width - 4
	if boxW < 30 {
		boxW = 30
	}
	innerW := boxW - 4

	var content strings.Builder
	content.WriteString(shellBoxStatusLine(name, args, 0, icon, dur, ""))
	content.WriteString("\n")

	var taskOrder []string
	switch name {
	case "discover_services":
		taskOrder = []string{"processes", "ports", "systemd", "docker"}
	case "system_info":
		taskOrder = systemInfoTaskOrder(args)
	case "network_info":
		taskOrder = []string{"interfaces", "routing", "arp"}
	}

	type taskState struct {
		status string
		info   string
	}
	tasks := make(map[string]*taskState)
	for _, t := range taskOrder {
		tasks[t] = &taskState{status: "pending"}
	}

	for _, line := range lines {
		if strings.Contains(line, "task_start") {
			if t := extractField(line, "task"); t != "" {
				if _, ok := tasks[t]; ok {
					tasks[t].status = "running"
				}
			}
		} else if strings.Contains(line, "task_done") {
			if t := extractField(line, "task"); t != "" {
				if _, ok := tasks[t]; ok {
					tasks[t].status = "done"
					count := extractField(line, "count")
					d := extractField(line, "dur")
					if count != "" {
						tasks[t].info = fmt.Sprintf("%s items found (%s)", count, d)
					} else {
						tasks[t].info = fmt.Sprintf("Completed (%s)", d)
					}
				}
			}
		} else if strings.Contains(line, "task_fail") {
			if t := extractField(line, "task"); t != "" {
				if _, ok := tasks[t]; ok {
					tasks[t].status = "fail"
					tasks[t].info = "Failed"
				}
			}
		} else if strings.Contains(line, "task_skip") {
			if t := extractField(line, "task"); t != "" {
				if _, ok := tasks[t]; ok {
					tasks[t].status = "skip"
				}
			}
		}
	}

	for _, t := range taskOrder {
		ts := tasks[t]
		if ts.status == "pending" && (t == "systemd" || t == "docker") {
			continue
		}
		if ts.status == "skip" {
			continue
		}

		var iconStr string
		var labelStyle lipgloss.Style
		switch ts.status {
		case "running":
			iconStr = StyleClaude().Render("●")
			labelStyle = StyleThinking
		case "done":
			iconStr = StyleSuccess.Render("\u221a")
			labelStyle = StyleCmdBoxDim
		case "fail":
			iconStr = StyleError.Render("x")
			labelStyle = StyleError
		default:
			iconStr = StyleCmdBoxDim.Render("○")
			labelStyle = StyleCmdBoxDim
		}

		displayName := strings.Title(t)
		statusText := taskStatusText(ts.status)
		row := fmt.Sprintf("  %s %-12s %s", iconStr, displayName, labelStyle.Render(statusText))
		if ts.info != "" {
			row += " " + labelStyle.Render("("+ts.info+")")
		}
		content.WriteString("\n" + truncateShellLine(row, innerW))
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(shellBoxBorderColor(icon)).
		PaddingLeft(1).
		PaddingRight(1).
		Width(boxW).
		Render(content.String())
}

func taskStatusText(status string) string {
	switch status {
	case "running":
		return "Running"
	case "done":
		return "Done"
	case "fail":
		return "Failed"
	default:
		return "Pending"
	}
}

// systemInfoTaskOrder는 args["sections"]를 파싱하여 실제 요청된 섹션 목록을 반환한다.
func systemInfoTaskOrder(args map[string]any) []string {
	all := []string{"os", "cpu", "memory", "disk", "uptime"}

	var raw string
	switch v := args["sections"].(type) {
	case string:
		raw = v
	case []interface{}:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				parts = append(parts, strings.TrimSpace(s))
			}
		}
		raw = strings.Join(parts, ",")
	}

	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" || raw == "all" {
		return all
	}

	var out []string
	for _, s := range strings.Split(raw, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return all
	}
	return out
}
