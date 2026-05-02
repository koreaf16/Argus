package tui

import (
	"fmt"
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
		SandboxValue:     "no sandbox",
		ModelValue:       emptyFallback(m.footer.Model, "unconfigured"),
		ContextValue:     emptyFallback(m.footer.ContextUsedLabel, "0% used"),
		MemoryValue:      emptyFallback(m.footer.MemoryUsageLabel, "-"),
		SessionValue:     emptyFallback(m.footer.SessionShortID, "-"),
		DiffValue:        diffValue,
		TokensValue:      tokensValue,
	}, m.theme.PromptTheme)

	if line := m.renderWorkflowLine(); line != "" {
		return line + "\n" + footer
	}
	return footer
}

// renderWorkflowLine 은 활성 워크플로우 카드가 있을 때 footer 위에 한 줄을
// 추가해 진행 단계를 표시한다. 없으면 빈 문자열을 반환한다.
func (m uiModel) renderWorkflowLine() string {
	phase := strings.TrimSpace(m.footer.WorkflowPhase)
	if phase == "" {
		return ""
	}
	category := strings.TrimSpace(m.footer.WorkflowCategory)
	if category == "" {
		category = "task"
	}
	progress := strings.TrimSpace(m.footer.WorkflowProgress)
	title := strings.TrimSpace(m.footer.WorkflowTitle)

	categoryLabel := strings.ToUpper(category[:1]) + category[1:]
	body := fmt.Sprintf("📋 %s · %s", categoryLabel, phase)
	if progress != "" {
		body += " " + progress
	}
	if title != "" {
		body += " — " + title
	}

	width := max(m.width, 2)
	body = " " + body + " "
	if lipgloss.Width(body) > width {
		// 필요 시 단축 — 진행도/단계 우선 보존, 제목 잘라냄.
		short := fmt.Sprintf(" 📋 %s · %s %s ", categoryLabel, phase, progress)
		if lipgloss.Width(short) > width {
			short = fmt.Sprintf(" 📋 %s ", phase)
		}
		body = short
	}
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.PromptTheme.FooterValueColor)).
		Bold(true)
	rendered := style.Render(body)
	if lipgloss.Width(rendered) < width {
		rendered += strings.Repeat(" ", width-lipgloss.Width(rendered))
	}
	return rendered
}

func (m uiModel) renderTranscriptModeFooter() string {
	mutedStyle := m.theme.style(m.theme.MutedColor)
	text := "  Showing detailed transcript · ctrl+o to toggle · ctrl+e to show all"
	w := m.width - lipgloss.Width(text)
	if w > 0 {
		text += strings.Repeat(" ", w)
	}
	return mutedStyle.Render(text)
}
