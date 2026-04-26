package markdown

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderCodeBlock renders a fenced code block with a language label header.
func RenderCodeBlock(lang string, lines []string, p Palette) string {
	label := "code"
	if strings.TrimSpace(lang) != "" {
		label = "code:" + strings.TrimSpace(lang)
	}

	var headStyle, lineStyle lipgloss.Style
	if p.DisableANSI {
		headStyle = lipgloss.NewStyle()
		lineStyle = lipgloss.NewStyle()
	} else {
		headStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.CodeHead))
		lineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.CodeLine))
	}

	out := make([]string, 0, len(lines)+2)
	out = append(out, headStyle.Render("[ "+label+" ]"))
	if len(lines) == 0 {
		out = append(out, lineStyle.Render("  "))
	}
	for _, line := range lines {
		out = append(out, lineStyle.Render("  "+line))
	}
	out = append(out, headStyle.Render("[ end ]"))
	return strings.Join(out, "\n")
}
