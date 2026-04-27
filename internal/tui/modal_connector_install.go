package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *uiModel) handleConnectorInstallKey(msg tea.KeyMsg) {
	st := m.modal.ConnectorInstall
	if st == nil {
		m.closeModal()
		return
	}
	if st.Installing {
		return
	}

	totalFields := len(st.Spec.EnvPrompts) + 1 // last index = Submit button

	switch msg.String() {
	case "tab", "down", "ctrl+n":
		st.FocusIdx = (st.FocusIdx + 1) % totalFields
	case "shift+tab", "up", "ctrl+p":
		st.FocusIdx = (st.FocusIdx - 1 + totalFields) % totalFields
	case "enter":
		if st.FocusIdx == len(st.Spec.EnvPrompts) {
			m.submitConnectorInstall()
			return
		}
		st.FocusIdx = (st.FocusIdx + 1) % totalFields
	case "esc":
		if m.modal.ConnectorInstallC != nil {
			m.modal.ConnectorInstallC <- connectorInstallResult{Cancelled: true}
		}
		m.closeModal()
	case "backspace":
		if st.FocusIdx < len(st.EnvValues) {
			v := st.EnvValues[st.FocusIdx]
			if len(v) > 0 {
				st.EnvValues[st.FocusIdx] = v[:len(v)-1]
			}
		}
	default:
		if st.FocusIdx < len(st.EnvValues) && len(msg.Runes) > 0 {
			st.EnvValues[st.FocusIdx] += string(msg.Runes)
		}
	}
}

func (m *uiModel) submitConnectorInstall() {
	st := m.modal.ConnectorInstall
	if st == nil {
		return
	}

	// Validate required fields
	for i, ep := range st.Spec.EnvPrompts {
		if ep.Required && strings.TrimSpace(st.EnvValues[i]) == "" {
			st.ErrMsg = ep.Key + " 값을 입력해주세요"
			st.FocusIdx = i
			return
		}
	}

	answers := make(map[string]string, len(st.Spec.EnvPrompts))
	for i, ep := range st.Spec.EnvPrompts {
		if v := strings.TrimSpace(st.EnvValues[i]); v != "" {
			answers[ep.Key] = v
		}
	}
	if m.modal.ConnectorInstallC != nil {
		m.modal.ConnectorInstallC <- connectorInstallResult{EnvAnswers: answers}
	}
	m.closeModal()
}
