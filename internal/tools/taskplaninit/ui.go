package taskplaninit

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/koreaf16/argus/internal/tui/toolui"
)

func init() {
	toolui.Register(ToolName, &Renderer{})
}

type Renderer struct{}

func (r *Renderer) CreateInteractiveModel(_ map[string]any, _ toolui.ThemeContext) toolui.InteractiveModel {
	return nil
}

func (r *Renderer) RenderToolUse(args map[string]any, _ string, theme toolui.ThemeContext) string {
	title, _ := args["title"].(string)
	category, _ := args["category"].(string)
	header := strings.TrimSpace(category)
	if header == "" {
		header = "task"
	}
	body := fmt.Sprintf("Initializing %s workflow: %s", header, strings.TrimSpace(title))
	return theme.Style(theme.ToolUseColor()).Bold(true).Render("  📋 ") +
		theme.Style(theme.BodyColor()).Render(body)
}

func (r *Renderer) RenderToolResult(resultText string, _ int64, theme toolui.ThemeContext) string {
	var res map[string]any
	if err := json.Unmarshal([]byte(resultText), &res); err != nil {
		return theme.Style(theme.StatusSuccessColor()).Render("  [workflow ready]")
	}
	phase, _ := res["phase"].(string)
	category, _ := res["category"].(string)
	phases, _ := res["phases"].([]any)
	total := len(phases)
	if total == 0 {
		total = 6
	}
	label := fmt.Sprintf("  [%s · phase=%s · %d steps queued]",
		strings.TrimSpace(category),
		strings.TrimSpace(phase),
		total,
	)
	return theme.Style(theme.StatusSuccessColor()).Render(label)
}
