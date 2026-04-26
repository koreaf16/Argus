package webfetch

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/koreaf16/argus/internal/tui/toolui"
)

// WebFetchRenderer provides custom UI for the webfetch tool.
type WebFetchRenderer struct{}

func init() {
	toolui.Register("webfetch", &WebFetchRenderer{})
}

func (r *WebFetchRenderer) CreateInteractiveModel(args map[string]any, theme toolui.ThemeContext) toolui.InteractiveModel {
	return nil
}

func (r *WebFetchRenderer) RenderToolUse(args map[string]any, _ string, theme toolui.ThemeContext) string {
	rawURL, _ := args["url"].(string)
	prompt, _ := args["prompt"].(string)

	titleStyle := theme.Style(theme.ToolUseColor()).Bold(true)
	bodyStyle := theme.Style(theme.BodyColor())
	mutedStyle := theme.Style(theme.MutedColor())

	var sb strings.Builder
	sb.WriteString(titleStyle.Render("  Fetch URL: "))
	sb.WriteString(bodyStyle.Render(rawURL))

	if strings.TrimSpace(prompt) != "" {
		sb.WriteString("\n")
		sb.WriteString(mutedStyle.Render("  Prompt:    "))

		promptSingle := strings.ReplaceAll(prompt, "\n", " ")
		if len(promptSingle) > 60 {
			promptSingle = promptSingle[:57] + "..."
		}
		sb.WriteString(mutedStyle.Render(promptSingle))
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

func (r *WebFetchRenderer) RenderToolResult(resultText string, durationMs int64, theme toolui.ThemeContext) string {
	var out fetchOutput
	if err := json.Unmarshal([]byte(resultText), &out); err != nil {
		return resultText
	}

	status := "OK"
	if out.Code != 200 && out.Code != 0 {
		status = fmt.Sprintf("%d %s", out.Code, out.CodeText)
	}

	sizeDisp := ""
	if out.Bytes > 1024*1024 {
		sizeDisp = fmt.Sprintf("%.1fMB", float64(out.Bytes)/(1024*1024))
	} else if out.Bytes > 1024 {
		sizeDisp = fmt.Sprintf("%.1fKB", float64(out.Bytes)/1024)
	} else {
		sizeDisp = fmt.Sprintf("%d bytes", out.Bytes)
	}

	durDisp := ""
	if durationMs > 0 {
		durDisp = fmt.Sprintf("%dms", durationMs)
	} else if out.DurationMS > 0 {
		durDisp = fmt.Sprintf("%dms", out.DurationMS)
	}

	msg := fmt.Sprintf("Received %s [%s]", sizeDisp, status)
	if durDisp != "" {
		msg += " in " + durDisp
	}

	return theme.Style(theme.ToolResultColor()).Render("  [done] " + msg)
}
