package tui

import (
	"strings"

	"github.com/muesli/reflow/ansi"
)

type footerState int

const (
	footerIdle footerState = iota
	footerBusy
	footerSelection
	footerSecretPrompt
	footerForm
	footerSlashMenu
)

func renderFooter(state footerState, width int) string {
	return buildFooterLine(footerHints(state), width)
}

func footerHints(state footerState) []string {
	switch state {
	case footerBusy:
		return []string{
			hintKey("Esc") + " cancel",
			hintKey("Ctrl+B") + " background",
			hintKey("Ctrl+L") + " redraw",
			hintKey("Ctrl+O") + " expand",
			hintKey("Enter") + " queue input",
		}
	case footerSelection:
		return []string{
			hintKey("Up/Down") + " move",
			hintKey("Enter") + " select",
			hintKey("Esc") + " cancel",
		}
	case footerSecretPrompt:
		return []string{
			hintKey("Enter") + " submit password",
			hintKey("Esc") + " cancel",
		}
	case footerForm:
		return []string{
			hintKey("Tab") + " next field",
			hintKey("Shift+Tab") + " prev field",
			hintKey("Enter") + " submit",
			hintKey("Esc") + " cancel",
		}
	case footerSlashMenu:
		return []string{
			hintKey("Up/Down") + " move",
			hintKey("Tab") + " accept",
			hintKey("Enter") + " submit",
			hintKey("Esc") + " close",
		}
	default:
		return []string{
			hintKey("Up/Down") + " history",
			hintKey("Tab") + " autocomplete",
			hintKey("Alt+Enter") + " newline",
			hintKey("Shift+Tab") + " plan",
			hintKey("Ctrl+T") + " tasks",
			hintKey("/help") + " commands",
		}
	}
}

func hintKey(key string) string {
	return StyleFooterHint.Bold(true).Render(key)
}

func buildFooterLine(hints []string, width int) string {
	sep := StyleFooterDim.Render(" | ")
	sepWidth := ansi.PrintableRuneWidth(sep)

	var parts []string
	totalWidth := 2
	for _, h := range hints {
		hWidth := ansi.PrintableRuneWidth(h)
		needed := hWidth
		if len(parts) > 0 {
			needed += sepWidth
		}
		if totalWidth+needed > width {
			break
		}
		parts = append(parts, h)
		totalWidth += needed
	}

	return "  " + strings.Join(parts, sep)
}
