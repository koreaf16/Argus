package promptinput

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type Suggestion struct {
	Command     string
	Description string
}

func renderFooterSuggestions(items []Suggestion, cursor, width int, theme Theme) string {
	if len(items) == 0 {
		return ""
	}

	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.SuggestionColor))
	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.SuggestionSelectedColor)).
		Bold(true)

	maxLines := len(items)
	if maxLines > 4 {
		maxLines = 4
	}

	lines := make([]string, 0, maxLines+1)
	for i := 0; i < maxLines; i++ {
		it := items[i]
		style := normalStyle
		if i == cursor {
			style = selectedStyle
		}
		plain := fmt.Sprintf("/%s  %s", it.Command, strings.TrimSpace(it.Description))
		if width > 0 && lipgloss.Width(plain) > width {
			plain = trimRunesToWidth(plain, width)
		}
		lines = append(lines, style.Render(plain))
	}

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.SuggestionHelpColor)).
		Render("tab accept | ctrl+n/ctrl+p move")
	lines = append(lines, help)
	return strings.Join(lines, "\n")
}

func trimRunesToWidth(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes)+"...") > width {
		runes = runes[:len(runes)-1]
	}
	if len(runes) == 0 {
		return ""
	}
	return string(runes) + "..."
}
