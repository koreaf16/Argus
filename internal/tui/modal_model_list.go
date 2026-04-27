package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *uiModel) handleModelListKey(msg tea.KeyMsg) {
	if m.modal.ModelList == nil {
		return
	}
	st := m.modal.ModelList
	switch msg.String() {
	case "up", "ctrl+p":
		if st.Cursor > 0 {
			st.Cursor--
		} else {
			st.Cursor = len(st.Entries) - 1
		}
	case "down", "ctrl+n":
		if st.Cursor < len(st.Entries)-1 {
			st.Cursor++
		} else {
			st.Cursor = 0
		}
	case "enter":
		if st.Cursor >= 0 && st.Cursor < len(st.Entries) {
			alias := st.Entries[st.Cursor].Alias
			if m.modal.ModelListC != nil {
				m.modal.ModelListC <- modelListResult{Alias: alias}
			}
			m.closeModal()
		}
	case "esc":
		if m.modal.ModelListC != nil {
			m.modal.ModelListC <- modelListResult{}
		}
		m.closeModal()
	}
}

func (m uiModel) renderModelList() string {
	if m.modal.ModelList == nil {
		return ""
	}
	st := m.modal.ModelList
	entries := st.Entries

	if len(entries) == 0 {
		return "No models registered."
	}

	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ToolUseTitleColor)).Bold(true)
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.AssistantMarkerColor)).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.AssistantMarkerColor)).Bold(true)
	bodyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.BodyColor))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.MutedColor))

	// 컬럼 폭 계산
	maxAlias := 15
	maxProvider := 12
	for _, e := range entries {
		if len(e.Alias) > maxAlias {
			maxAlias = len(e.Alias)
		}
		if len(e.Provider) > maxProvider {
			maxProvider = len(e.Provider)
		}
	}

	header := fmt.Sprintf("   %-8s %-*s %-*s %-s", "[Active]", maxAlias, "Alias", maxProvider, "Provider", "Model ID")
	var sb strings.Builder
	sb.WriteString(headerStyle.Render(header))
	sb.WriteString("\n")
	sb.WriteString(mutedStyle.Render("   " + strings.Repeat("─", m.width-12)))
	sb.WriteString("\n")

	for i, e := range entries {
		activeMark := "○"
		if e.Active {
			activeMark = "●"
		}
		cursorMark := "  "
		style := bodyStyle
		if i == st.Cursor {
			cursorMark = "❯ "
			style = selectedStyle
		} else if e.Active {
			style = activeStyle
		}

		if e.Status != "ready" && i != st.Cursor {
			style = mutedStyle
		}

		line := fmt.Sprintf("%s%-8s %-*s %-*s %-s",
			cursorMark, activeMark, maxAlias, e.Alias, maxProvider, e.Provider, e.ModelID)
		sb.WriteString(style.Render(line))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	help := mutedStyle.Render("   [↑/↓] 이동  [Enter] 선택/전환  [Esc] 닫기")
	sb.WriteString(help)

	// renderModal 내부의 border/padding 시스템과 조화를 이루기 위해 박스 스타일을 직접 반환하지 않고
	// 내부 내용물만 반환하거나, renderModal의 틀 안에 맞춥니다.
	return sb.String()
}
