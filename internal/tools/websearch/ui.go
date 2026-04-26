package websearch

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/koreaf16/argus/internal/tui/toolui"
)

// WebSearchRenderer provides custom UI for the web_search tool.
type WebSearchRenderer struct{}

func init() {
	toolui.Register("web_search", &WebSearchRenderer{})
}

func (r *WebSearchRenderer) CreateInteractiveModel(args map[string]any, theme toolui.ThemeContext) toolui.InteractiveModel {
	return nil
}

func (r *WebSearchRenderer) RenderToolUse(args map[string]any, _ string, theme toolui.ThemeContext) string {
	query, _ := args["query"].(string)
	if strings.TrimSpace(query) == "" {
		query = strings.TrimSpace(fmt.Sprintf("%v", args["query"]))
	}

	allowed := toStringSlice(args["allowed_domains"])
	blocked := toStringSlice(args["blocked_domains"])

	titleStyle := theme.Style(theme.ToolUseColor()).Bold(true)
	bodyStyle := theme.Style(theme.BodyColor())
	mutedStyle := theme.Style(theme.MutedColor())

	var sb strings.Builder
	sb.WriteString(titleStyle.Render("  Keywords: "))
	sb.WriteString(bodyStyle.Render(fmt.Sprintf("\"%s\"", query)))

	if len(allowed) > 0 {
		sb.WriteString("\n")
		sb.WriteString(mutedStyle.Render(fmt.Sprintf("             site:%s", strings.Join(allowed, ", "))))
	}
	if len(blocked) > 0 {
		sb.WriteString("\n")
		sb.WriteString(mutedStyle.Render(fmt.Sprintf("             -site:%s", strings.Join(blocked, ", "))))
	}

	boxW := theme.Width() - 4
	if boxW < 30 {
		boxW = 30
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.BorderColor())).
		PaddingRight(1).
		Width(boxW).
		Render(sb.String())
}

func (r *WebSearchRenderer) RenderToolResult(resultText string, durationMs int64, theme toolui.ThemeContext) string {
	var out SearchOutput
	if err := json.Unmarshal([]byte(resultText), &out); err != nil {
		return resultText
	}

	resultCount := 0
	for _, res := range out.Results {
		if m, ok := res.(map[string]any); ok {
			if content, ok := m["content"].([]any); ok {
				resultCount = len(content)
			}
		}
	}

	durDisp := ""
	if durationMs > 0 {
		if durationMs >= 1000 {
			durDisp = fmt.Sprintf("%.2fs", float64(durationMs)/1000)
		} else {
			durDisp = fmt.Sprintf("%dms", durationMs)
		}
	} else if out.DurationSeconds > 0 {
		if out.DurationSeconds >= 1 {
			durDisp = fmt.Sprintf("%.2fs", out.DurationSeconds)
		} else {
			durDisp = fmt.Sprintf("%.0fms", out.DurationSeconds*1000)
		}
	}

	msg := fmt.Sprintf("Did 1 search with %d results", resultCount)
	if durDisp != "" {
		msg += " in " + durDisp
	}

	return theme.Style(theme.ToolResultColor()).Render("  [done] " + msg)
}

// Legacy helpers kept for compatibility with existing callers.
func RenderToolUseMessage(query string, allowedDomains, blockedDomains []string, verbose bool) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	msg := fmt.Sprintf("\"%s\"", query)
	if !verbose {
		return msg
	}
	if len(allowedDomains) > 0 {
		msg += fmt.Sprintf(", only allowing domains: %s", strings.Join(allowedDomains, ", "))
	}
	if len(blockedDomains) > 0 {
		msg += fmt.Sprintf(", blocking domains: %s", strings.Join(blockedDomains, ", "))
	}
	return msg
}

func RenderWebSearchProgress(query string) string {
	return fmt.Sprintf(" web_search: %s", strings.TrimSpace(query))
}

func RenderResultSummary(resultCount int, durationSeconds float64) string {
	if durationSeconds >= 1 {
		return fmt.Sprintf("Did 1 search with %d results in %.0fs", resultCount, durationSeconds)
	}
	return fmt.Sprintf("Did 1 search with %d results in %.0fms", resultCount, durationSeconds*1000)
}

func toStringSlice(v any) []string {
	switch raw := v.(type) {
	case nil:
		return nil
	case []string:
		out := make([]string, 0, len(raw))
		for _, s := range raw {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			s := strings.TrimSpace(fmt.Sprintf("%v", item))
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
