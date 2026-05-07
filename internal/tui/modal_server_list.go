package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/koreaf16/argus/internal/presentation"
	"github.com/koreaf16/argus/internal/services/inventory"
	"github.com/koreaf16/argus/internal/services/workspace"
)

// visibleActions returns the action list for the given server status.
func visibleActions(st workspace.StatusEntry) []serverAction {
	if st.Kind == workspace.ServerKindLocal {
		return nil
	}
	actions := make([]serverAction, 0, 4)
	if st.Connected {
		actions = append(actions, actionDisconnect)
	} else {
		actions = append(actions, actionConnect)
	}
	if st.Kind != workspace.ServerKindAccount {
		actions = append(actions, actionEdit)
	}
	actions = append(actions, actionDelete)
	return actions
}

// handleServerListKey handles key events for the server list panel.
func (m *uiModel) handleServerListKey(msg tea.KeyMsg) {
	sl := m.modal.ServerList
	if sl == nil {
		return
	}

	// PendingDelete confirmation mode.
	if sl.PendingDelete != "" {
		switch msg.String() {
		case "y", "Y":
			m.confirmDeleteServer(sl.PendingDelete)
		case "n", "N", "esc":
			sl.PendingDelete = ""
		}
		return
	}

	// Action sub-menu mode.
	if sl.ShowAction {
		m.handleServerActionKey(msg)
		return
	}

	// List navigation mode.
	maxIdx := len(sl.Entries) // last row is "Add New Server"
	switch msg.String() {
	case "esc":
		m.cancelServerList()
	case "up", "ctrl+p":
		if sl.Cursor > 0 {
			sl.Cursor--
		} else {
			sl.Cursor = maxIdx
		}
	case "down", "ctrl+n":
		if sl.Cursor < maxIdx {
			sl.Cursor++
		} else {
			sl.Cursor = 0
		}
	case "a":
		m.closeServerListWith(serverListResult{Action: "add"})
	case "enter":
		if sl.Cursor == maxIdx {
			// "Add New Server" row
			m.closeServerListWith(serverListResult{Action: "add"})
			return
		}
		st := sl.Entries[sl.Cursor]
		if st.Kind == workspace.ServerKindLocal {
			// local workspace: no actions available
			return
		}
		sl.ShowAction = true
		sl.Action = serverActionState{
			Alias:  st.Alias,
			Cursor: 0,
		}
	}
}

// handleServerActionKey handles key events for the action sub-menu.
func (m *uiModel) handleServerActionKey(msg tea.KeyMsg) {
	sl := m.modal.ServerList
	if sl == nil {
		return
	}

	var st workspace.StatusEntry
	for _, e := range sl.Entries {
		if e.Alias == sl.Action.Alias {
			st = e
			break
		}
	}
	actions := visibleActions(st)
	maxIdx := len(actions) - 1
	if maxIdx < 0 {
		sl.ShowAction = false
		return
	}

	switch msg.String() {
	case "esc":
		sl.ShowAction = false
		sl.ErrorMsg = ""
	case "up", "ctrl+p":
		if sl.Action.Cursor > 0 {
			sl.Action.Cursor--
		} else {
			sl.Action.Cursor = maxIdx
		}
	case "down", "ctrl+n":
		if sl.Action.Cursor < maxIdx {
			sl.Action.Cursor++
		} else {
			sl.Action.Cursor = 0
		}
	case "enter":
		if sl.Action.Cursor >= 0 && sl.Action.Cursor < len(actions) {
			m.executeServerAction(actions[sl.Action.Cursor])
		}
	case "c", "C":
		for i, a := range actions {
			if a == actionConnect {
				sl.Action.Cursor = i
				m.executeServerAction(actionConnect)
				return
			}
		}
	case "d", "D":
		for i, a := range actions {
			if a == actionDisconnect {
				sl.Action.Cursor = i
				m.executeServerAction(actionDisconnect)
				return
			}
		}
	case "e", "E":
		m.executeServerAction(actionEdit)
	case "x", "X":
		m.executeServerAction(actionDelete)
	}
}

// executeServerAction runs the given action for the currently selected server.
func (m *uiModel) executeServerAction(action serverAction) {
	sl := m.modal.ServerList
	if sl == nil {
		return
	}
	alias := sl.Action.Alias

	switch action {
	case actionConnect:
		ws := m.app.cfg.Workspace
		if ws == nil {
			return
		}
		// 패널을 먼저 닫아야 패스워드 모달이 즉시 뜬다.
		m.closeServerListWith(serverListResult{Action: "closed"})
		go func() {
			taskID := fmt.Sprintf("modal-sc-%d", time.Now().UnixNano())
			// EventToolUse: ServerConnectRenderer가 카드 UI를 그리도록 한다.
			m.app.send(presentationEventMsg{Event: presentation.Event{
				Kind:     presentation.EventToolUse,
				ToolName: "server_connect",
				Input:    fmt.Sprintf(`{"server":%q}`, alias),
				TaskID:   taskID,
			}})

			resolvedAlias, err := ws.ConnectAndActivate(alias)
			if err != nil {
				errText := fmt.Sprintf("연결 실패: %v", err)
				m.app.send(presentationEventMsg{Event: presentation.Event{
					Kind:   presentation.EventToolDelta,
					Text:   errText,
					TaskID: taskID,
				}})
				m.app.send(presentationEventMsg{Event: presentation.Event{
					Kind:     presentation.EventToolResult,
					ToolName: "server_connect",
					Text:     errText,
					TaskID:   taskID,
				}})
				return
			}

			if m.app.cfg.State != nil {
				m.app.cfg.State.SetActiveWorkspace(ws.ActiveAlias())
			}
			m.app.send(footerStateMsg{
				Footer: presentation.BuildFooterState(m.app.cfg.State, m.app.cfg.WorkDir),
			})

			// Connected 마커 + inspect 진행 중
			m.app.send(presentationEventMsg{Event: presentation.Event{
				Kind:   presentation.EventToolDelta,
				Text:   fmt.Sprintf("[ARGUS_SERVER_CONNECT:connected]\n%s에 연결되었습니다. 환경 정보를 확인 중입니다...\n", resolvedAlias),
				TaskID: taskID,
			}})
			snap, inspectErr := ws.RunInspect(m.app.ctx, resolvedAlias)
			if inspectErr == nil {
				m.app.send(presentationEventMsg{Event: presentation.Event{
					Kind:   presentation.EventToolDelta,
					Text:   workspace.FormatInspectSummary(snap),
					TaskID: taskID,
				}})
			}

			// 동기 인벤토리 스캔 (inventoryChannel 사용, PTY lane 점유 없음)
			if resolvedAlias != workspace.LocalAlias {
				m.app.send(presentationEventMsg{Event: presentation.Event{
					Kind:   presentation.EventToolDelta,
					Text:   "[ARGUS_INVENTORY_SCANNING]\n",
					TaskID: taskID,
				}})
				invSnap, invErr := ws.RescanInventory(m.app.ctx, resolvedAlias)
				if invErr == nil {
					cardLines := inventory.FormatUICard(invSnap)
					m.app.send(presentationEventMsg{Event: presentation.Event{
						Kind:   presentation.EventToolDelta,
						Text:   fmt.Sprintf("[ARGUS_INVENTORY_READY:%d]\n%s", invSnap.DurationMs, strings.Join(cardLines, "\n")),
						TaskID: taskID,
					}})
				}
			}

			// 성공 결과 (SetResult가 마커를 제거하므로 빈 결과로 처리됨)
			m.app.send(presentationEventMsg{Event: presentation.Event{
				Kind:     presentation.EventToolResult,
				ToolName: "server_connect",
				Text:     "[ARGUS_SERVER_CONNECT:connected]",
				TaskID:   taskID,
			}})
			m.app.send(m.app.buildFooterMsg())
		}()

	case actionDisconnect:
		ws := m.app.cfg.Workspace
		if ws == nil {
			return
		}
		// 패널을 먼저 닫아야 즉각 반응이 가능하다.
		m.closeServerListWith(serverListResult{Action: "closed"})
		go func() {
			wasActive := ws.ActiveAlias() == alias
			err := ws.Disconnect(alias)
			if err != nil {
				m.app.send(presentationEventMsg{Event: presentation.Event{
					Kind: presentation.EventSystem,
					Text: fmt.Sprintf("disconnect %s failed: %v", alias, err),
				}})
				return
			}

			if wasActive {
				if setErr := ws.SetActive(workspace.LocalAlias); setErr != nil {
					m.app.send(presentationEventMsg{Event: presentation.Event{
						Kind: presentation.EventSystem,
						Text: fmt.Sprintf("disconnected %s but failed to switch workspace: %v", alias, setErr),
					}})
					return
				}
				if m.app.cfg.State != nil {
					m.app.cfg.State.SetActiveWorkspace(ws.ActiveAlias())
				}
				m.app.send(footerStateMsg{
					Footer: presentation.BuildFooterState(m.app.cfg.State, m.app.cfg.WorkDir),
				})
				m.app.send(presentationEventMsg{Event: presentation.Event{
					Kind: presentation.EventSystem,
					Text: fmt.Sprintf("disconnected %s, reverted to local workspace", alias),
				}})
				return
			}

			m.app.send(presentationEventMsg{Event: presentation.Event{
				Kind: presentation.EventSystem,
				Text: fmt.Sprintf("disconnected: %s", alias),
			}})
		}()

	case actionEdit:
		m.closeServerListWith(serverListResult{Action: "edit", EditAlias: alias})

	case actionDelete:
		sl.ShowAction = false
		sl.PendingDelete = alias
	}
}

// confirmDeleteServer performs the actual deletion after y/n confirmation.
func (m *uiModel) confirmDeleteServer(alias string) {
	sl := m.modal.ServerList
	if sl == nil {
		return
	}
	sl.PendingDelete = ""
	sl.ErrorMsg = ""

	ws := m.app.cfg.Workspace
	if ws == nil {
		return
	}
	if err := ws.Registry().Remove(alias); err != nil {
		sl.ErrorMsg = fmt.Sprintf("delete %s: %v", alias, err)
		return
	}
	_ = ws.Registry().Save()
	_ = ws.Disconnect(alias)
	if m.app.cfg.Credentials != nil {
		m.app.cfg.Credentials.Delete(alias)
		_ = m.app.cfg.Credentials.Save()
	}
	m.refreshServerListEntries()
}

// cancelServerList closes the server list panel without action.
func (m *uiModel) cancelServerList() {
	m.closeServerListWith(serverListResult{Action: "closed"})
}

// closeServerListWith sends the result and closes the modal.
func (m *uiModel) closeServerListWith(res serverListResult) {
	if m.modal.ServerListC != nil {
		m.modal.ServerListC <- res
	}
	m.closeModal()
}

// refreshServerListEntries re-snapshots workspace status and clamps cursor.
func (m *uiModel) refreshServerListEntries() {
	sl := m.modal.ServerList
	if sl == nil {
		return
	}
	if m.app.cfg.Workspace != nil {
		sl.Entries = m.app.cfg.Workspace.Status()
	}
	max := len(sl.Entries)
	if sl.Cursor > max {
		sl.Cursor = max
	}
}
