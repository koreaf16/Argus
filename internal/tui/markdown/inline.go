package markdown

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// inlineRe matches inline markdown tokens in order of precedence (longest first).
var inlineRe = regexp.MustCompile(
	`\*\*\*.+?\*\*\*` + `|` +
		`\*\*.+?\*\*` + `|` +
		`\*.+?\*` + `|` +
		`_.+?_` + `|` +
		`~~.+?~~` + `|` +
		`\[[^\]]+\]\([^)]+\)` + `|` +
		"`[^`]+`" + `|` +
		`<u>.+?</u>` + `|` +
		`https?://\S+`,
)

var linkExtractRe = regexp.MustCompile(`^\[([^\]]+)\]\(([^)]+)\)$`)

// ParseInline converts inline markdown in text to ANSI-styled output.
// Returns plain text when p.DisableANSI is true.
func ParseInline(text string, p Palette) string {
	if p.DisableANSI || text == "" {
		return text
	}
	return parseWith(text, p.Body, false, false, p)
}

// ParseInlineWithBase converts inline markdown with a caller-supplied base color and bold flag.
// Used by the table renderer to apply per-cell base styling while still parsing inline tokens.
// Returns stripped plain text when p.DisableANSI is true.
func ParseInlineWithBase(text, baseColor string, bold bool, p Palette) string {
	if p.DisableANSI {
		return StripInlineMarkers(text)
	}
	return parseWith(text, baseColor, bold, false, p)
}

// StripInlineMarkers removes inline markdown syntax and returns the plain display text.
// Used for width calculation so marker characters don't inflate column sizes.
func StripInlineMarkers(text string) string {
	return inlineRe.ReplaceAllStringFunc(text, func(token string) string {
		switch {
		case strings.HasPrefix(token, "***") && strings.HasSuffix(token, "***") && len(token) > 6:
			return token[3 : len(token)-3]
		case strings.HasPrefix(token, "**") && strings.HasSuffix(token, "**") && len(token) > 4:
			return token[2 : len(token)-2]
		case len(token) > 2 &&
			((token[0] == '*' && token[len(token)-1] == '*') ||
				(token[0] == '_' && token[len(token)-1] == '_')):
			return token[1 : len(token)-1]
		case strings.HasPrefix(token, "~~") && strings.HasSuffix(token, "~~") && len(token) > 4:
			return token[2 : len(token)-2]
		case token[0] == '`':
			return strings.Trim(token, "`")
		case token[0] == '[':
			if m := linkExtractRe.FindStringSubmatch(token); m != nil {
				return m[1] + " (" + m[2] + ")"
			}
			return token
		case strings.HasPrefix(token, "<u>") && strings.HasSuffix(token, "</u>"):
			return token[3 : len(token)-4]
		default:
			return token
		}
	})
}

func parseWith(text, color string, bold, italic bool, p Palette) string {
	if text == "" {
		return ""
	}
	locs := inlineRe.FindAllStringIndex(text, -1)
	if len(locs) == 0 {
		return seg(text, color, bold, italic)
	}
	var sb strings.Builder
	prev := 0
	for _, loc := range locs {
		s, e := loc[0], loc[1]
		if s > prev {
			sb.WriteString(seg(text[prev:s], color, bold, italic))
		}
		sb.WriteString(applyToken(text[s:e], color, bold, italic, p))
		prev = e
	}
	if prev < len(text) {
		sb.WriteString(seg(text[prev:], color, bold, italic))
	}
	return sb.String()
}

// seg renders plain text with given style flags.
func seg(text, color string, bold, italic bool) string {
	s := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
	if bold {
		s = s.Bold(true)
	}
	if italic {
		s = s.Italic(true)
	}
	return s.Render(text)
}

func applyToken(token, color string, bold, italic bool, p Palette) string {
	switch {
	// ***bold+italic***
	case strings.HasPrefix(token, "***") && strings.HasSuffix(token, "***") && len(token) > 6:
		return parseWith(token[3:len(token)-3], color, true, true, p)

	// **bold**
	case strings.HasPrefix(token, "**") && strings.HasSuffix(token, "**") && len(token) > 4:
		return parseWith(token[2:len(token)-2], color, true, italic, p)

	// *italic* or _italic_
	case len(token) > 2 &&
		((token[0] == '*' && token[len(token)-1] == '*') ||
			(token[0] == '_' && token[len(token)-1] == '_')):
		return parseWith(token[1:len(token)-1], color, bold, true, p)

	// ~~strikethrough~~
	case strings.HasPrefix(token, "~~") && strings.HasSuffix(token, "~~") && len(token) > 4:
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Strike)).
			Strikethrough(true).
			Render(token[2 : len(token)-2])

	// `inline code`
	case token[0] == '`':
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.InlineCode)).
			Render(strings.Trim(token, "`"))

	// [text](url)
	case token[0] == '[':
		if m := linkExtractRe.FindStringSubmatch(token); m != nil {
			linkText := parseWith(m[1], color, bold, italic, p)
			base := seg(" (", color, bold, italic)
			url := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Link)).Render(m[2])
			return linkText + base + url + seg(")", color, bold, italic)
		}
		return seg(token, color, bold, italic)

	// <u>underline</u>
	case strings.HasPrefix(token, "<u>") && strings.HasSuffix(token, "</u>"):
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(color)).
			Underline(true).
			Render(token[3 : len(token)-4])

	// raw URL
	case strings.HasPrefix(token, "http://") || strings.HasPrefix(token, "https://"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Link)).Render(token)

	default:
		return seg(token, color, bold, italic)
	}
}
