package promptinput

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var ansiRegex = regexp.MustCompile("[\u001b\u009b][[()#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-7,A-ORZcf-nqry=><]")

func stripANSI(str string) string {
	return ansiRegex.ReplaceAllString(str, "")
}

const (
	promptLineIndicator = "> "
	wrappedLineIndent   = "  "
)

func RenderBaseTextInput(inner string, width int) string {
	return RenderBaseTextInputWithTheme(inner, width, ResolveTheme("default"))
}

func RenderBaseTextInputWithTheme(inner string, width int, theme Theme) string {
	if width <= 0 {
		width = 80
	}
	if width < 20 {
		width = 20
	}

	lines := strings.Split(inner, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}

	hasBg := theme.InputBoxBg != ""
	bg := lipgloss.Color(theme.InputBoxBg)

	indicatorChar := promptLineIndicator
	indicatorColorKey := theme.PromptIndicatorColor
	if theme.IndicatorChar != "" {
		indicatorChar = theme.IndicatorChar + " "
		if theme.IndicatorColor != "" {
			indicatorColorKey = theme.IndicatorColor
		}
	}

	indicatorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(indicatorColorKey)).
		Bold(true)
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.PromptTextColor))

	if hasBg {
		indicatorStyle = indicatorStyle.Background(bg)
		textStyle = textStyle.Background(bg)
	}

	formatted := make([]string, 0, len(lines))
	for i := range lines {
		prefix := wrappedLineIndent
		if i == 0 {
			prefix = indicatorChar
		}

		// STRIP ANSI sequences from the text to prevent it from resetting our background
		plainText := stripANSI(lines[i])
		var line string
		if hasBg {
			bg := lipgloss.Color(theme.InputBoxBg)
			lineStyle := lipgloss.NewStyle().Background(bg)

			// content = prefix + text (줄 시작의 공백 제거하여 왼쪽으로 한 칸 당김)
			plainLine := prefix + plainText
			padding := width - lipgloss.Width(plainLine)

			// Pre-render parts and combine to ensure background color persists after resets
			line = indicatorStyle.Render(prefix) + textStyle.Render(plainText)
			if padding > 0 {
				line += lineStyle.Render(strings.Repeat(" ", padding))
			}
		} else {
			// 줄 시작의 공백 제거
			line = indicatorStyle.Render(prefix) + textStyle.Render(lines[i])
			line = fitLineWidth(line, width)
		}
		formatted = append(formatted, line)
	}

	topStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.InputBoxBg))
	bottomStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.InputBoxBg))

	// Ensure borders fit the width
	topBorder := topStyle.Render(strings.Repeat("▄", width))
	bottomBorder := bottomStyle.Render(strings.Repeat("▀", width))

	return strings.Join([]string{topBorder, strings.Join(formatted, "\n"), bottomBorder}, "\n")
}

func fitLineWidth(line string, width int) string {
	if width <= 0 {
		return line
	}
	if lipgloss.Width(line) > width {
		return trimToWidth(line, width)
	}
	gap := width - lipgloss.Width(line)
	if gap <= 0 {
		return line
	}
	return line + strings.Repeat(" ", gap)
}

func trimToWidth(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes)) > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes)
}
