package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/koreaf16/argus/internal/services/workspace"
)

// renderServerList renders the interactive server list panel.
func (m uiModel) renderServerList(
	titleStyle, accentStyle, mutedStyle lipgloss.Style,
	borderStyle lipgloss.Style,
) string {
	sl := m.modal.ServerList
	if sl == nil {
		return borderStyle.Render(titleStyle.Render("Servers") + "\n(state missing)")
	}

	successStyle := m.theme.style(m.theme.UserTitleColor)
	errorStyle := m.theme.style(m.theme.ErrorTitleColor)

	lines := []string{titleStyle.Render("Servers"), ""}

	if sl.ShowAction {
		lines = append(lines, m.renderServerActionMenu(sl, accentStyle, mutedStyle, errorStyle)...)
	} else {
		lines = append(lines, m.renderServerListRows(sl, accentStyle, mutedStyle, successStyle)...)
	}

	return borderStyle.Render(strings.Join(lines, "\n"))
}

// renderServerListRows renders the server list rows plus the "Add New" row.
func (m uiModel) renderServerListRows(
	sl *serverListState,
	accentStyle, mutedStyle, successStyle lipgloss.Style,
) []string {
	errorStyle := m.theme.style(m.theme.ErrorTitleColor)
	var lines []string

	if len(sl.Entries) == 0 {
		lines = append(lines, mutedStyle.Render("  (no servers configured)"))
	}

	for i, st := range sl.Entries {
		isCursor := sl.Cursor == i
		lines = append(lines, m.renderServerRow(st, isCursor, accentStyle, mutedStyle, successStyle))
	}

	// 디자인 시스템 v2.1: 투박한 구분선 제거 후 여백으로 대체
	lines = append(lines, "")

	// "Add New Server" row
	addRow := "  + Add New Server"
	if sl.Cursor == len(sl.Entries) {
		addRow = m.theme.ChevronStyle.Render("› Add New Server")
	} else {
		addRow = mutedStyle.Render(addRow)
	}
	lines = append(lines, addRow)

	// PendingDelete confirmation
	if sl.PendingDelete != "" {
		lines = append(lines, "")
		lines = append(lines, errorStyle.Render(fmt.Sprintf(
			`  Delete %q?  y. yes  n. no`, sl.PendingDelete,
		)))
	} else if sl.ErrorMsg != "" {
		lines = append(lines, "")
		lines = append(lines, errorStyle.Render("  ✗ "+sl.ErrorMsg))
	}

	lines = append(lines, "")
	lines = append(lines, mutedStyle.Render("  ↑↓ navigate  Enter select  a add  Esc close"))
	return lines
}

// renderServerRow renders a single server row.
func (m uiModel) renderServerRow(
	st workspace.StatusEntry,
	isCursor bool,
	accentStyle, mutedStyle, successStyle lipgloss.Style,
) string {
	// Status dot
	statusDot := mutedStyle.Render("•")
	if st.Connected {
		statusDot = successStyle.Render("•")
	}

	// Kind Icon
	icon := "🌐"
	if st.Kind == workspace.ServerKindLocal {
		icon = "🏠"
	}

	// Host info
	hostInfo := ""
	if st.Kind != workspace.ServerKindLocal && st.User != "" {
		hostInfo = "  " + mutedStyle.Render(st.User+"@...")
	}

	alias := fmt.Sprintf("%-14s", st.Alias)
	row := fmt.Sprintf("%s %s %s %s", statusDot, icon, alias, hostInfo)

	if isCursor {
		// 슬라이싱(row[2:])을 제거하고 처음부터 다시 구성하여 ANSI 코드 파손 방지
		return m.theme.ChevronStyle.Render("› ") + lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.BodyColor)).Bold(true).Render(fmt.Sprintf("%s %s %s", icon, alias, hostInfo))
	}
	return "  " + row
}

// renderServerActionMenu renders the action sub-menu for a selected server.
func (m uiModel) renderServerActionMenu(
	sl *serverListState,
	accentStyle, mutedStyle, errorStyle lipgloss.Style,
) []string {
	var st workspace.StatusEntry
	for _, e := range sl.Entries {
		if e.Alias == sl.Action.Alias {
			st = e
			break
		}
	}

	actions := visibleActions(st)
	lines := []string{
		accentStyle.Render("  Actions for: " + sl.Action.Alias),
		"",
	}

	labels := map[serverAction]string{
		actionConnect:    "[c] Connect",
		actionDisconnect: "[d] Disconnect",
		actionEdit:       "[e] Edit",
		actionDelete:     "[x] Delete",
	}

	for i, a := range actions {
		label, ok := labels[a]
		if !ok {
			continue
		}
		if sl.Action.Cursor == i {
			lines = append(lines, accentStyle.Bold(true).Render("> "+label))
		} else {
			lines = append(lines, "    "+label)
		}
	}

	if sl.ErrorMsg != "" {
		lines = append(lines, "")
		lines = append(lines, errorStyle.Render("  ✗ "+sl.ErrorMsg))
	}

	lines = append(lines, "")
	lines = append(lines, mutedStyle.Render("  ↑↓ navigate  Enter select  Esc back"))
	return lines
}
