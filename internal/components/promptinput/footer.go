package promptinput

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type FooterConfig struct {
	Width            int
	ShowSuggestions  bool
	Suggestions      []Suggestion
	SuggestionCursor int

	WorkspaceValue string
	SandboxValue   string
	ModelValue     string
	ContextValue   string
	MemoryValue    string
	SessionValue   string
	DiffValue      string
	TokensValue    string
}

var (
	footerSandboxErrorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	footerSandboxWarnStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
)

func RenderFooter(cfg FooterConfig) string {
	return RenderFooterWithTheme(cfg, ResolveTheme("default"))
}

func RenderFooterWithTheme(cfg FooterConfig, theme Theme) string {
	lines := make([]string, 0, 4)
	if cfg.ShowSuggestions && len(cfg.Suggestions) > 0 {
		if suggestions := renderFooterSuggestions(cfg.Suggestions, cfg.SuggestionCursor, cfg.Width, theme); suggestions != "" {
			lines = append(lines, suggestions)
		}
	}

	labels := []string{
		"workspace (/directory)",
		"sandbox",
		"/model",
		"context",
		"memory",
		"session",
		"diff",
		"tokens",
	}
	values := []string{
		emptyDefault(cfg.WorkspaceValue, "~"),
		emptyDefault(cfg.SandboxValue, "no sandbox"),
		emptyDefault(shortenModelDisplay(cfg.ModelValue), "unconfigured"),
		emptyDefault(cfg.ContextValue, "0% used"),
		emptyDefault(cfg.MemoryValue, "-"),
		emptyDefault(cfg.SessionValue, "-"),
		emptyDefault(cfg.DiffValue, "-"),
		emptyDefault(cfg.TokensValue, "-"),
	}

	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.FooterLabelColor))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.FooterValueColor))
	valueStyles := []lipgloss.Style{
		valueStyle,
		sandboxStyleFor(values[1], valueStyle),
		valueStyle,
		valueStyle,
		valueStyle,
		valueStyle,
		valueStyle,
		valueStyle,
	}

	labelRow, valueRow := renderFooterGrid(labels, values, labelStyle, valueStyles, cfg.Width)
	lines = append(lines, labelRow, valueRow)
	return strings.Join(lines, "\n")
}

func renderFooterGrid(labels, values []string, labelStyle lipgloss.Style, valueStyles []lipgloss.Style, width int) (string, string) {
	count := len(labels)
	if len(values) < count {
		count = len(values)
	}
	if count == 0 {
		return "", ""
	}

	effectiveWidth := width
	if width >= 2 {
		effectiveWidth = width - 2
	}
	labels, values, valueStyles = foldFooterColumns(labels, values, valueStyles, effectiveWidth)
	count = len(labels)
	if count == 0 {
		return "", ""
	}

	colWidths := make([]int, count)
	contentWidth := 0
	for i := 0; i < count; i++ {
		w := max(lipgloss.Width(labels[i]), lipgloss.Width(values[i]))
		if w < 6 {
			w = 6
		}
		colWidths[i] = w
		contentWidth += w
	}

	// 가변 간격 계산
	gap := 1
	if count > 1 {
		gap = (effectiveWidth - contentWidth) / (count - 1)
		if gap < 2 {
			gap = 2
		} else if gap > 12 {
			gap = 12 // 너무 벌어지지 않게 제한
		}
	}

	total := contentWidth + gap*(count-1)
	if effectiveWidth > 0 && total > effectiveWidth {
		over := total - effectiveWidth
		// 우선순위 낮은 컬럼부터 축소
		candidates := []int{4, 5, 6, 7, 3, 1, 2, 0}
		order := make([]int, 0, len(candidates))
		for _, idx := range candidates {
			if idx >= 0 && idx < count {
				order = append(order, idx)
			}
		}
		for _, idx := range order {
			for over > 0 && colWidths[idx] > 6 {
				colWidths[idx]--
				over--
			}
			if over == 0 {
				break
			}
		}
		// 간격도 줄임
		for over > 0 && gap > 1 {
			gap--
			over -= (count - 1)
		}
	}

	sep := strings.Repeat(" ", max(1, gap))
	labelParts := make([]string, count)
	valueParts := make([]string, count)
	for i := 0; i < count; i++ {
		lplain := trimToWidthEllipsis(labels[i], colWidths[i])
		vplain := trimToWidthEllipsis(values[i], colWidths[i])
		labelParts[i] = labelStyle.Render(padRight(lplain, colWidths[i]))
		valueParts[i] = valueStyles[i].Render(padRight(vplain, colWidths[i]))
	}

	labelRow := strings.Join(labelParts, sep)
	valueRow := strings.Join(valueParts, sep)
	if width >= 2 {
		labelRow = " " + labelRow + " "
		valueRow = " " + valueRow + " "
	}

	// Pad to full width
	if lipgloss.Width(labelRow) < width {
		labelRow += strings.Repeat(" ", width-lipgloss.Width(labelRow))
	}
	if lipgloss.Width(valueRow) < width {
		valueRow += strings.Repeat(" ", width-lipgloss.Width(valueRow))
	}

	return labelRow, valueRow
}

func foldFooterColumns(labels, values []string, valueStyles []lipgloss.Style, effectiveWidth int) ([]string, []string, []lipgloss.Style) {
	if len(labels) == 0 || len(values) == 0 || len(valueStyles) == 0 {
		return labels, values, valueStyles
	}

	visible := []int{0, 1, 2, 3, 4, 5, 6, 7}
	switch {
	case effectiveWidth < 68:
		visible = []int{0, 1, 2}
	case effectiveWidth < 84:
		visible = []int{0, 1, 2, 3}
	case effectiveWidth < 100:
		visible = []int{0, 1, 2, 3, 4}
	case effectiveWidth < 120:
		visible = []int{0, 1, 2, 3, 4, 5}
	}

	outLabels := make([]string, 0, len(visible))
	outValues := make([]string, 0, len(visible))
	outStyles := make([]lipgloss.Style, 0, len(visible))
	for _, idx := range visible {
		if idx < 0 || idx >= len(labels) || idx >= len(values) || idx >= len(valueStyles) {
			continue
		}
		outLabels = append(outLabels, labels[idx])
		outValues = append(outValues, values[idx])
		outStyles = append(outStyles, valueStyles[idx])
	}
	if len(outLabels) == 0 {
		return labels, values, valueStyles
	}
	return outLabels, outValues, outStyles
}

func sandboxStyleFor(v string, defaultStyle lipgloss.Style) lipgloss.Style {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "no sandbox":
		return footerSandboxErrorStyle
	case "untrusted", "all tools", "current process":
		return footerSandboxWarnStyle
	default:
		return defaultStyle
	}
}

func shortenModelDisplay(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return model
	}
	if idx := strings.LastIndex(model, "/"); idx >= 0 {
		model = model[idx+1:]
	}
	if model == "" {
		return model
	}
	suffixes := []string{"-4bit", "-8bit", "-awq", "-gptq", "-gguf", "-it"}
	for {
		lower := strings.ToLower(model)
		trimmed := false
		for _, s := range suffixes {
			if strings.HasSuffix(lower, s) {
				candidate := model[:len(model)-len(s)]
				if candidate == "" {
					return model
				}
				model = candidate
				trimmed = true
				break
			}
		}
		if !trimmed {
			break
		}
	}
	return model
}

func emptyDefault(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
}

func padRight(s string, width int) string {
	if width <= 0 {
		return s
	}
	gap := width - lipgloss.Width(s)
	if gap <= 0 {
		return s
	}
	return s + strings.Repeat(" ", gap)
}

func padLeft(s string, width int) string {
	if width <= 0 {
		return s
	}
	gap := width - lipgloss.Width(s)
	if gap <= 0 {
		return s
	}
	return strings.Repeat(" ", gap) + s
}

func trimToWidthEllipsis(s string, width int) string {
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
