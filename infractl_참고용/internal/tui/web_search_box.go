// Package tui
// File: web_search_box.go
// Description: web_search / discover_web_servers 도구의 전용 박스 렌더러
// Responsibility: Engine/Keywords/태스크 상태를 통합 렌더링

package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/yourorg/infractl/internal/web"
)

type webSearchTask struct {
	host   string
	status string // "running", "done", "fail"
	dur    string
}

// renderWebSearchBox는 실행 중/완료 후 공통으로 web_search 전용 박스를 렌더링한다.
func renderWebSearchBox(args map[string]any, outputLines []string, icon boxIcon, dur time.Duration, width int) string {
	boxW := width - 4
	if boxW < 30 {
		boxW = 30
	}
	innerW := boxW - 4

	var content strings.Builder
	content.WriteString(shellBoxStatusLine("web_search", args, 0, icon, dur, ""))

	// Engine 행
	content.WriteString("\n" + truncateShellLine(
		StyleCmdBoxDim.Render("  Engine:   ")+webSearchEngineLabel(),
		innerW,
	))

	// Keywords 행 (쿼리 + domain 필터)
	keywords := buildWebSearchKeywords(args)
	if len(keywords) > 0 {
		content.WriteString("\n" + truncateShellLine(
			StyleCmdBoxDim.Render("  Keywords: ")+StyleClaude().Render(keywords[0]),
			innerW,
		))
		for _, extra := range keywords[1:] {
			content.WriteString("\n" + truncateShellLine(
				StyleCmdBoxDim.Render("             ")+StyleClaude().Render(extra),
				innerW,
			))
		}
	}

	// 태스크 행 또는 결과 요약 행
	if isWebSearchTaskOutput(outputLines) {
		tasks := parseWebSearchTaskLines(outputLines)
		for _, t := range tasks {
			content.WriteString("\n" + truncateShellLine(renderWebSearchTaskLine(t), innerW))
		}
	} else {
		for _, line := range outputLines {
			content.WriteString("\n" + truncateShellLine(StyleCmdBoxDim.Render("  ")+line, innerW))
		}
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSubtle).
		PaddingLeft(1).
		PaddingRight(1).
		Width(boxW).
		Render(content.String())
}

// webSearchEngineLabel은 "SearXNG · 192.168.0.3:30080" 형태의 엔진 레이블을 반환한다.
func webSearchEngineLabel() string {
	host := web.SearXNGBaseURL
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	return StyleClaude().Render("SearXNG · " + host)
}

// buildWebSearchKeywords는 query + site:/(-site:) 필터 조합을 줄 단위로 반환한다.
// 첫 줄: 기본 쿼리, 이후 줄: 도메인 필터 (있는 경우).
func buildWebSearchKeywords(args map[string]any) []string {
	query, _ := args["query"].(string)
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}

	lines := []string{query}
	for _, d := range webSearchDomainArgs(args, "allowed_domains") {
		lines = append(lines, "site:"+d)
	}
	for _, d := range webSearchDomainArgs(args, "blocked_domains") {
		lines = append(lines, "-site:"+d)
	}
	return lines
}

func webSearchDomainArgs(args map[string]any, key string) []string {
	switch v := args[key].(type) {
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// isWebSearchTaskOutput은 outputLines가 task_start/done/fail 형식인지 판단한다.
func isWebSearchTaskOutput(lines []string) bool {
	for _, line := range lines {
		if strings.Contains(line, "task_start") || strings.Contains(line, "task_done") || strings.Contains(line, "task_fail") {
			return true
		}
	}
	return false
}

// parseWebSearchTaskLines는 task_start/done/fail EmitOutput 행을 파싱한다.
func parseWebSearchTaskLines(lines []string) []webSearchTask {
	order := []string{}
	tasks := map[string]*webSearchTask{}

	for _, line := range lines {
		host := extractField(line, "task")
		if host == "" {
			continue
		}
		if _, exists := tasks[host]; !exists {
			tasks[host] = &webSearchTask{host: host, status: "running"}
			order = append(order, host)
		}
		t := tasks[host]
		switch {
		case strings.Contains(line, "task_done"):
			t.status = "done"
			t.dur = extractField(line, "dur")
		case strings.Contains(line, "task_fail"):
			t.status = "fail"
			t.dur = extractField(line, "dur")
		}
	}

	result := make([]webSearchTask, 0, len(order))
	for _, h := range order {
		result = append(result, *tasks[h])
	}
	return result
}

func renderWebSearchTaskLine(t webSearchTask) string {
	var iconStr string
	var info string
	switch t.status {
	case "running":
		iconStr = StyleClaude().Render("  ●")
		info = StyleThinking.Render("fetching...")
	case "done":
		iconStr = StyleSuccess.Render("  \u221a")
		info = StyleCmdBoxDim.Render(fmt.Sprintf("done (%s)", t.dur))
	case "fail":
		iconStr = StyleError.Render("  \u00d7")
		info = StyleError.Render(fmt.Sprintf("failed (%s)", t.dur))
	default:
		iconStr = StyleCmdBoxDim.Render("  ○")
		info = StyleCmdBoxDim.Render("pending")
	}
	return fmt.Sprintf("%s %-28s %s", iconStr, t.host, info)
}

// renderDiscoverWebBox는 discover_web_servers 도구의 서브태스크 상태를 박스로 렌더링한다.
func renderDiscoverWebBox(name string, args map[string]any, lines []string, icon boxIcon, dur time.Duration, width int) string {
	boxW := width - 4
	if boxW < 30 {
		boxW = 30
	}
	innerW := boxW - 4

	var content strings.Builder
	content.WriteString(shellBoxStatusLine(name, args, 0, icon, dur, ""))
	content.WriteString("\n")

	type taskState struct {
		status string
		info   string
	}
	tasks := make(map[string]*taskState)
	taskOrder := []string{"web_servers", "web_ports", "ssl_certs", "vhosts"}
	for _, t := range taskOrder {
		tasks[t] = &taskState{status: "pending"}
	}

	for _, line := range lines {
		if strings.Contains(line, "task_start") {
			if t := extractField(line, "task"); t != "" {
				tasks[t].status = "running"
			}
		} else if strings.Contains(line, "task_done") {
			if t := extractField(line, "task"); t != "" {
				tasks[t].status = "done"
				count := extractField(line, "count")
				d := extractField(line, "dur")
				tasks[t].info = fmt.Sprintf("Found %s items (%s)", count, d)
			}
		} else if strings.Contains(line, "task_fail") {
			if t := extractField(line, "task"); t != "" {
				tasks[t].status = "fail"
				tasks[t].info = "Scan failed"
			}
		}
	}

	for _, t := range taskOrder {
		ts := tasks[t]
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

		displayName := strings.Title(strings.ReplaceAll(t, "_", " "))
		statusText := taskStatusText(ts.status)
		row := fmt.Sprintf("  %s %-14s %s", iconStr, displayName, labelStyle.Render(statusText))
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
