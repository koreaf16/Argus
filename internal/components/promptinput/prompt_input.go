package promptinput

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type PromptConfig struct {
	Width      int
	InputView  string
	FooterView FooterConfig
	Theme      Theme
}

func RenderPrompt(cfg PromptConfig) string {
	theme := cfg.Theme
	if strings.TrimSpace(theme.Name) == "" {
		theme = ResolveTheme("default")
	}
	return RenderPromptWithTheme(cfg, theme)
}

func RenderPromptWithTheme(cfg PromptConfig, theme Theme) string {
	parts := []string{
		RenderTextInputWithTheme(cfg.InputView, cfg.Width, theme),
		RenderFooterWithTheme(cfg.FooterView, theme),
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
