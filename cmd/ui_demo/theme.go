package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type uiVariant string

const (
	variantCurrent        uiVariant = "current"
	variantArgusSignature uiVariant = "argus-signature"
	variantMinimalPro     uiVariant = "minimal-pro"
)

type demoTheme struct {
	Variant uiVariant
	ANSI    bool

	Accent   string
	Accent2  string
	Success  string
	Warning  string
	Danger   string
	Info     string
	Text     string
	Muted    string
	PanelBG  string
}

func parseVariant(raw string) (uiVariant, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "", "argus-signature", "signature", "argus":
		return variantArgusSignature, nil
	case "current":
		return variantCurrent, nil
	case "minimal", "minimal-pro":
		return variantMinimalPro, nil
	default:
		return "", fmt.Errorf("unknown variant %q (use current|argus-signature|minimal-pro)", raw)
	}
}

func nextVariant(v uiVariant) uiVariant {
	switch v {
	case variantCurrent:
		return variantArgusSignature
	case variantArgusSignature:
		return variantMinimalPro
	default:
		return variantCurrent
	}
}

func themeFor(v uiVariant, ansi bool) demoTheme {
	t := demoTheme{
		Variant: v,
		ANSI:    ansi,
	}
	switch v {
	case variantCurrent:
		t.Accent = "#A06EE1"
		t.Accent2 = "#4285F4"
		t.Success = "#34A853"
		t.Warning = "#FBBC04"
		t.Danger = "#EA4335"
		t.Info = "#4FB3BF"
		t.Text = "#FFFFFF"
		t.Muted = "#AFAFAF"
		t.PanelBG = "#262626"
	case variantMinimalPro:
		t.Accent = "#D0D0D0"
		t.Accent2 = "#A8A8A8"
		t.Success = "#C8C8C8"
		t.Warning = "#B8B8B8"
		t.Danger = "#E0E0E0"
		t.Info = "#B0B0B0"
		t.Text = "#F0F0F0"
		t.Muted = "#8A8A8A"
		t.PanelBG = "#1E1E1E"
	default:
		t.Accent = "#F49B4F"
		t.Accent2 = "#5CC4D6"
		t.Success = "#61CF8E"
		t.Warning = "#F5C451"
		t.Danger = "#D96570"
		t.Info = "#4492E6"
		t.Text = "#F9FAFB"
		t.Muted = "#9AA0A6"
		t.PanelBG = "#1D1F24"
	}
	return t
}

func paint(ansi bool, text, fg, bg string, bold, italic bool) string {
	s := lipgloss.NewStyle().Bold(bold).Italic(italic)
	if ansi {
		if fg != "" {
			s = s.Foreground(lipgloss.Color(fg))
		}
		if bg != "" {
			s = s.Background(lipgloss.Color(bg))
		}
	}
	return s.Render(text)
}

func accentCard(width int, t demoTheme, borderColor, title string, lines ...string) string {
	head := paint(t.ANSI, title, borderColor, "", true, false)
	body := strings.Join(lines, "\n")
	content := head
	if body != "" {
		content += "\n" + body
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		PaddingLeft(1).
		MaxWidth(width)
	if t.ANSI {
		style = style.BorderForeground(lipgloss.Color(borderColor))
	}
	return style.Render(content)
}

func panelLine(width int, t demoTheme, prefix, text string) string {
	left := paint(t.ANSI, prefix, t.Accent, t.PanelBG, true, false)
	right := paint(t.ANSI, text, t.Text, t.PanelBG, false, false)
	line := left + right
	return fitLine(line, width, t)
}

func fitLine(line string, width int, t demoTheme) string {
	if width <= 0 {
		return line
	}
	w := lipgloss.Width(line)
	if w == width {
		return line
	}
	if w > width {
		return trimToWidth(line, width)
	}
	return line + strings.Repeat(" ", width-w)
}

func trimToWidth(s string, width int) string {
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
