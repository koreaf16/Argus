package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/koreaf16/argus/internal/components/promptinput"
)

func (m uiModel) renderFooter() string {
	if m.transcriptMode {
		return m.renderTranscriptModeFooter()
	}

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

	diffValue := formatDiff(m.footer.DiffAdded, m.footer.DiffRemoved)

	// busy 중에는 footer 토큰 영역을 비운다 — 토큰 카운터는 상단 Thinking 라인이 담당.
	tokensValue := ""
	if !m.busy {
		tokensValue = formatTokensDetailed(m.footer.TokensIn, m.footer.TokensOut, m.footer.TokensThinking)
	}

	footer := promptinput.RenderFooterWithTheme(promptinput.FooterConfig{
		Width:            m.width,
		ShowSuggestions:  m.showSlashOverlay(),
		Suggestions:      suggestions,
		SuggestionCursor: m.slashCursor,
		WorkspaceValue:   workspaceValue,
		SandboxValue:     "샌드박스 없음",
		ModelValue:       emptyFallback(m.footer.Model, "설정되지 않음"),
		ContextValue:     emptyFallback(m.footer.ContextUsedLabel, "0% 사용됨"),
		MemoryValue:      emptyFallback(m.footer.MemoryUsageLabel, "-"),
		SessionValue:     emptyFallback(m.footer.SessionShortID, "-"),
		DiffValue:        diffValue,
		TokensValue:      tokensValue,
	}, m.theme.PromptTheme)

	return footer
}

func (m uiModel) renderTranscriptModeFooter() string {
	mutedStyle := m.theme.style(m.theme.MutedColor)
	text := "  상세 트랜스크립트 표시 중 · ctrl+o 토글 · ctrl+e 전체 표시"
	w := m.width - lipgloss.Width(text)
	if w > 0 {
		text += strings.Repeat(" ", w)
	}
	return mutedStyle.Render(text)
}
