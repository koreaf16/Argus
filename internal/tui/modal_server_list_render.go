package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/koreaf16/argus/internal/services/workspace"
)

const modalWidth = 60

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

	// Header with Horizontal Line
	headerText := fmt.Sprintf(" HOSTS / TARGETS (%d) ", len(sl.Entries))
	lineLen := modalWidth - len(headerText) - 2
	if lineLen < 0 {
		lineLen = 0
	}
	header := titleStyle.Render(headerText) + mutedStyle.Render(strings.Repeat("─", lineLen))

	lines := []string{header, ""}

	if sl.ShowAction {
		lines = append(lines, m.renderServerActionMenu(sl, accentStyle, mutedStyle, errorStyle)...)
	} else {
		lines = append(lines, m.renderServerListRows(sl, accentStyle, mutedStyle, successStyle)...)
	}

	return borderStyle.Width(modalWidth).Render(strings.Join(lines, "\n"))
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

	// Group by local, physical SSH hosts, and work account targets.
	var localEntries []int
	var hostEntries []int
	accountsByParent := make(map[string][]int)
	var orphanAccounts []int
	for i, st := range sl.Entries {
		if st.Kind == workspace.ServerKindLocal {
			localEntries = append(localEntries, i)
		} else if st.Kind == workspace.ServerKindAccount {
			if strings.TrimSpace(st.ParentAlias) == "" {
				orphanAccounts = append(orphanAccounts, i)
			} else {
				accountsByParent[st.ParentAlias] = append(accountsByParent[st.ParentAlias], i)
			}
		} else {
			hostEntries = append(hostEntries, i)
		}
	}

	// 1. Current Workspace (Local)
	if len(localEntries) > 0 {
		lines = append(lines, mutedStyle.Bold(true).Render("  CURRENT WORKSPACE"))
		for _, i := range localEntries {
			lines = append(lines, m.renderServerRow(sl.Entries[i], sl.Cursor == i, accentStyle, mutedStyle, successStyle))
		}
		lines = append(lines, "")
	}

	// 2. Hosts and their work account targets.
	if len(hostEntries) > 0 {
		lines = append(lines, mutedStyle.Bold(true).Render("  HOSTS"))
		for _, i := range hostEntries {
			lines = append(lines, m.renderServerRow(sl.Entries[i], sl.Cursor == i, accentStyle, mutedStyle, successStyle))
			for _, accountIdx := range accountsByParent[sl.Entries[i].Alias] {
				lines = append(lines, m.renderServerRow(sl.Entries[accountIdx], sl.Cursor == accountIdx, accentStyle, mutedStyle, successStyle))
			}
		}
		lines = append(lines, "")
	}
	if len(orphanAccounts) > 0 {
		lines = append(lines, mutedStyle.Bold(true).Render("  WORK ACCOUNTS"))
		for _, i := range orphanAccounts {
			lines = append(lines, m.renderServerRow(sl.Entries[i], sl.Cursor == i, accentStyle, mutedStyle, successStyle))
		}
		lines = append(lines, "")
	}

	// Footer Separator
	lines = append(lines, mutedStyle.Render("  "+strings.Repeat("─", modalWidth-4)))

	// "Add New Server" row
	addRow := "    + Add New Server"
	if sl.Cursor == len(sl.Entries) {
		addRow = m.theme.ChevronStyle.Render("  › ") + accentStyle.Bold(true).Render("Add New Server")
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

	lines = append(lines, "", mutedStyle.Render("  ↑↓ navigate  Enter select  a add  Esc close"))
	return lines
}

// renderServerRow renders a single server row with right-aligned status.
func (m uiModel) renderServerRow(
	st workspace.StatusEntry,
	isCursor bool,
	accentStyle, mutedStyle, successStyle lipgloss.Style,
) string {
	// Status dot and text
	statusDot := mutedStyle.Render("○")
	statusText := "OFFLINE"
	if st.Connected {
		statusDot = successStyle.Render("●")
		statusText = "CONNECTED"
		if st.Kind == workspace.ServerKindLocal {
			statusText = "READY"
		}
	}

	// Kind Icon
	icon := "🌐"
	if st.Kind == workspace.ServerKindLocal {
		icon = "🏠"
	}

	if st.Kind == workspace.ServerKindAccount {
		icon = "?궇"
	}

	// Left part: [status dot] [icon] [alias] [user/host]
	aliasStr := st.Alias
	if st.Kind == workspace.ServerKindLocal {
		aliasStr = "[" + st.Alias + "]"
	}

	info := ""
	if st.Kind == workspace.ServerKindAccount {
		targetUser := strings.TrimSpace(st.TargetUser)
		if targetUser == "" {
			targetUser = strings.TrimSpace(st.User)
		}
		info = mutedStyle.Render(targetUser + " via " + st.ParentAlias)
	} else if st.Kind != workspace.ServerKindLocal {
		if st.User != "" {
			info = mutedStyle.Render(st.User + "@...")
		}
	} else {
		info = mutedStyle.Render("(Local machine)")
	}

	// Layout using lipgloss for alignment
	leftContent := fmt.Sprintf("%s %s %-14s %s", statusDot, icon, aliasStr, info)
	if isCursor {
		leftContent = m.theme.ChevronStyle.Render("› ") + lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.BodyColor)).Bold(true).Render(fmt.Sprintf("%s %-14s %s", icon, aliasStr, info))
	} else {
		leftContent = "  " + leftContent
	}

	// Status text alignment
	rightContent := statusText
	if st.Connected {
		rightContent = successStyle.Render(statusText)
	} else {
		rightContent = mutedStyle.Render(statusText)
	}

	// Calculate padding for right alignment
	totalWidth := modalWidth - 4
	leftLen := lipgloss.Width(leftContent)
	rightLen := lipgloss.Width(rightContent)
	padding := totalWidth - leftLen - rightLen
	if padding < 1 {
		padding = 1
	}

	return leftContent + strings.Repeat(" ", padding) + rightContent
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
			lines = append(lines, m.theme.ChevronStyle.Render("  › ")+accentStyle.Bold(true).Render(label))
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
