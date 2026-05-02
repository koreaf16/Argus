package servermetrics

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/koreaf16/argus/internal/services/workspace"
	"github.com/koreaf16/argus/internal/tui/toolui"
)

func init() {
	toolui.Register("server_metrics", &ServerMetricsRenderer{})
}

type ServerMetricsRenderer struct{}

func (r *ServerMetricsRenderer) RenderToolUse(args map[string]any, _ string, theme toolui.ThemeContext) string {
	alias, _ := args["server"].(string)
	alias = strings.TrimSpace(alias)
	if alias == "" {
		alias = "active"
	}
	return toolui.FormatToolCall("ServerMetrics", alias, 80, theme)
}

func (r *ServerMetricsRenderer) RenderToolResult(resultText string, _ int64, theme toolui.ThemeContext) string {
	if strings.TrimSpace(resultText) == "" {
		return ""
	}
	var snap workspace.MetricsSnapshot
	if err := json.Unmarshal([]byte(resultText), &snap); err != nil {
		lines := splitNonEmpty(resultText)
		return toolui.FormatResultLines(lines, true, false, theme)
	}
	return renderMetricsSnap(snap, theme)
}

func (r *ServerMetricsRenderer) CreateInteractiveModel(_ map[string]any, _ toolui.ThemeContext) toolui.InteractiveModel {
	return nil
}

func renderMetricsSnap(snap workspace.MetricsSnapshot, theme toolui.ThemeContext) string {
	var lines []string

	// 라인 1: alias — CPU  MEM  DISK
	var row1 []string
	if v := firstLine(snap.CPU); v != "" {
		row1 = append(row1, "CPU: "+v)
	}
	if v := firstLine(snap.Memory); v != "" {
		row1 = append(row1, "MEM: "+v)
	}
	if v := firstLine(snap.Disk); v != "" {
		row1 = append(row1, "DISK: "+v)
	}
	header := strings.TrimSpace(snap.Alias)
	if len(row1) > 0 {
		joined := strings.Join(row1, "  ")
		if header != "" {
			header += "  —  " + joined
		} else {
			header = joined
		}
	}
	if header != "" {
		lines = append(lines, header)
	}

	// 라인 2: load · uptime
	var row2 []string
	if v := firstLine(snap.Load); v != "" {
		row2 = append(row2, "load: "+v)
	}
	if v := firstLine(snap.Uptime); v != "" {
		row2 = append(row2, "uptime: "+v)
	}
	if len(row2) > 0 {
		lines = append(lines, strings.Join(row2, "  ·  "))
	}

	// GPU (있을 때만)
	if v := firstLine(snap.GPU); v != "" {
		lines = append(lines, "GPU: "+v)
	}

	// 오류 (정렬된 순서로 표시)
	if len(snap.Errors) > 0 {
		keys := make([]string, 0, len(snap.Errors))
		for k := range snap.Errors {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			lines = append(lines, "! "+k+": "+snap.Errors[k])
		}
	}

	if len(lines) == 0 {
		return toolui.FormatStatusLine("no metrics collected", theme)
	}
	// 오류만 있는 경우 isError=true, 아니면 false
	isErrorOnly := len(snap.Errors) > 0 && len(lines) == len(snap.Errors)
	return toolui.FormatResultLines(lines, true, isErrorOnly, theme)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return s
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
