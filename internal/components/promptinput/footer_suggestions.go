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
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.SuggestionHelpColor))

	maxLines := len(items)
	if maxLines > 6 {
		maxLines = 6
	}

	maxCmdLen := 0
	for i := 0; i < maxLines; i++ {
		l := lipgloss.Width(items[i].Command)
		if l > maxCmdLen {
			maxCmdLen = l
		}
	}

	lines := make([]string, 0, maxLines)
	for i := 0; i < maxLines; i++ {
		it := items[i]
		cmdStr := it.Command
		cmdPad := maxCmdLen - lipgloss.Width(cmdStr)
		if cmdPad > 0 {
			cmdStr += strings.Repeat(" ", cmdPad)
		}

		var line string
		if i == cursor {
			line = selectedStyle.Render(fmt.Sprintf(" ❯ /%s  ", cmdStr)) + descStyle.Render(it.Description)
		} else {
			line = normalStyle.Render(fmt.Sprintf("   /%s  ", cmdStr)) + descStyle.Render(it.Description)
		}
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.SuggestionColor)).
		Padding(0, 1)

	if width > 0 {
		boxStyle = boxStyle.MaxWidth(width - 2)
	}

	return boxStyle.Render(content)
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
