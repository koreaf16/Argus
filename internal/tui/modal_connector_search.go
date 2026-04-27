package tui

import tea "github.com/charmbracelet/bubbletea"

func (m *uiModel) handleConnectorSearchKey(msg tea.KeyMsg) {
	st := m.modal.ConnectorSearch
	if st == nil {
		m.closeModal()
		return
	}

	switch msg.String() {
	case "up", "ctrl+p":
		if st.Cursor > 0 {
			st.Cursor--
		} else if len(st.Results) > 0 {
			st.Cursor = len(st.Results) - 1
		}
	case "down", "ctrl+n":
		if st.Cursor < len(st.Results)-1 {
			st.Cursor++
		} else {
			st.Cursor = 0
		}
	case "enter":
		if len(st.Results) > 0 {
			selected := st.Results[st.Cursor]
			if m.modal.ConnectorSearchC != nil {
				m.modal.ConnectorSearchC <- connectorSearchResult{Spec: &selected}
			}
		} else {
			if m.modal.ConnectorSearchC != nil {
				m.modal.ConnectorSearchC <- connectorSearchResult{}
			}
		}
		m.closeModal()
	case "esc":
		if m.modal.ConnectorSearchC != nil {
			m.modal.ConnectorSearchC <- connectorSearchResult{}
		}
		m.closeModal()
	}
}
