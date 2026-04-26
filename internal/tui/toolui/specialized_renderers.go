package toolui

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// RenderDiff renders a unified diff with colors.
func RenderDiff(diff string, width int, theme ThemeContext) string {
	if strings.TrimSpace(diff) == "" {
		return ""
	}

	lines := strings.Split(diff, "\n")
	var rendered []string

	plusStyle := theme.Style(theme.StatusSuccessColor())
	minusStyle := theme.Style(theme.StatusErrorColor())
	headerStyle := theme.Style(theme.ToolResultColor()).Bold(true)
	mutedStyle := theme.Style(theme.MutedColor())

	for _, line := range lines {
		if width > 0 && len(line) > width {
			line = line[:width-3] + "..."
		}

		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			rendered = append(rendered, plusStyle.Render(line))
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			rendered = append(rendered, minusStyle.Render(line))
		case strings.HasPrefix(line, "@@"):
			rendered = append(rendered, headerStyle.Render(line))
		case strings.HasPrefix(line, "---"), strings.HasPrefix(line, "+++"):
			rendered = append(rendered, mutedStyle.Render(line))
		default:
			rendered = append(rendered, theme.Style(theme.BodyColor()).Render(line))
		}
	}

	return strings.Join(rendered, "\n")
}

// RenderCode highlights code using Chroma.
func RenderCode(code, language string, width int, theme ThemeContext) string {
	lexer := lexers.Get(language)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	style := styles.Get("monokai") // Gemini-like dark theme
	if style == nil {
		style = styles.Fallback
	}

	formatter := formatters.Get("terminal256")
	if formatter == nil {
		formatter = formatters.Fallback
	}

	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return code
	}

	var buf strings.Builder
	err = formatter.Format(&buf, style, iterator)
	if err != nil {
		return code
	}

	return buf.String()
}
