package tui

import (
	"strings"

	"github.com/koreaf16/argus/internal/components/promptinput"
)

func (m uiModel) renderFooter() string {
	suggestions := make([]promptinput.Suggestion, 0, len(m.slashMatches))
	for _, item := range m.slashMatches {
		suggestions = append(suggestions, promptinput.Suggestion{
			Command:     item.Command,
			Description: item.Description,
		})
	}

	alias := strings.TrimSpace(m.footer.Workspace)
	if alias == "" {
		alias = "local"
	}
	cwd := strings.TrimSpace(m.footer.CWD)
	if cwd == "" || cwd == "." {
		cwd = "/"
	}
	workspaceValue := "[" + alias + "] " + cwd

	return promptinput.RenderFooterWithTheme(promptinput.FooterConfig{
		Width:            m.width,
		ShowSuggestions:  m.showSlashOverlay(),
		Suggestions:      suggestions,
		SuggestionCursor: m.slashCursor,
		WorkspaceValue:   workspaceValue,
		SandboxValue:     "no sandbox",
		ModelValue:       emptyFallback(m.footer.Model, "unconfigured"),
		ContextValue:     emptyFallback(m.footer.ContextUsedLabel, "0% used"),
		MemoryValue:      emptyFallback(m.footer.MemoryUsageLabel, "-"),
		SessionValue:     emptyFallback(m.footer.SessionShortID, "-"),
		DiffValue:        formatDiff(m.footer.DiffAdded, m.footer.DiffRemoved),
		TokensValue:      formatTokens(m.footer.TokensUsed),
	}, m.theme.PromptTheme)
}
