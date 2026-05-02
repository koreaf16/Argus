package serverinspect

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/koreaf16/argus/internal/services/workspace"
	"github.com/koreaf16/argus/internal/tui/toolui"
)

func init() {
	toolui.Register("server_inspect", &ServerInspectRenderer{})
}

type ServerInspectRenderer struct{}

func (r *ServerInspectRenderer) RenderToolUse(args map[string]any, _ string, theme toolui.ThemeContext) string {
	alias, _ := args["server"].(string)
	alias = strings.TrimSpace(alias)
	if alias == "" {
		alias = "active"
	}
	return toolui.FormatToolCall("ServerInspect", alias, 80, theme)
}

func (r *ServerInspectRenderer) RenderToolResult(resultText string, _ int64, theme toolui.ThemeContext) string {
	if strings.TrimSpace(resultText) == "" {
		return ""
	}
	var snap workspace.InspectSnapshot
	if err := json.Unmarshal([]byte(resultText), &snap); err != nil {
		// JSON 파싱 실패 시 평문 폴백
		lines := splitNonEmpty(resultText)
		return toolui.FormatResultLines(lines, true, false, theme)
	}
	return renderInspectSnap(snap, theme)
}

func (r *ServerInspectRenderer) CreateInteractiveModel(_ map[string]any, _ toolui.ThemeContext) toolui.InteractiveModel {
	return nil
}

func renderInspectSnap(snap workspace.InspectSnapshot, theme toolui.ThemeContext) string {
	var lines []string

	// 라인 1: alias — OS (kernel)
	header := strings.TrimSpace(snap.Alias)
	if os := firstLine(snap.OS); os != "" {
		detail := os
		if kernel := firstLine(snap.Kernel); kernel != "" && !strings.Contains(os, kernel) {
			detail += " (" + kernel + ")"
		}
		if header != "" {
			header += "  —  " + detail
		} else {
			header = detail
		}
	}
	if header != "" {
		lines = append(lines, header)
	}

	// 라인 2: user · shell · uptime
	var row2 []string
	if v := strings.TrimSpace(snap.User); v != "" {
		row2 = append(row2, "user: "+v)
	}
	if v := firstLine(snap.Shell); v != "" {
		row2 = append(row2, "shell: "+v)
	}
	if v := strings.TrimSpace(snap.Uptime); v != "" {
		row2 = append(row2, "uptime: "+v)
	}
	if len(row2) > 0 {
		lines = append(lines, strings.Join(row2, "  ·  "))
	}

	// 라인 3: mem · disk
	var row3 []string
	if v := firstLine(snap.Memory); v != "" {
		row3 = append(row3, "mem: "+v)
	}
	if v := firstLine(snap.Disk); v != "" {
		row3 = append(row3, "disk: "+v)
	}
	if len(row3) > 0 {
		lines = append(lines, strings.Join(row3, "  ·  "))
	}

	// 라인 4: services · ports · containers
	var row4 []string
	if snap.Services != "" {
		row4 = append(row4, fmt.Sprintf("%d services", countLines(snap.Services)))
	}
	if snap.Listeners != "" {
		row4 = append(row4, fmt.Sprintf("%d listening ports", countLines(snap.Listeners)))
	}
	if snap.Docker != "" {
		row4 = append(row4, fmt.Sprintf("%d docker containers", countLines(snap.Docker)))
	}
	if len(row4) > 0 {
		lines = append(lines, strings.Join(row4, "  ·  "))
	}

	if len(lines) == 0 {
		return toolui.FormatStatusLine("no data collected", theme)
	}
	return toolui.FormatResultLines(lines, true, false, theme)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return s
}

func countLines(s string) int {
	return len(strings.Split(strings.TrimRight(s, "\n"), "\n"))
}

func splitNonEmpty(s string) []string {
	var out []string
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if l := strings.TrimSpace(line); l != "" {
			out = append(out, l)
		}
	}
	return out
}
