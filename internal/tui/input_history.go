package tui

func (m *uiModel) pushInputHistory(text string) {
	if text == "" {
		return
	}
	n := len(m.inputHistory)
	if n > 0 && m.inputHistory[n-1] == text {
		return
	}
	m.inputHistory = append(m.inputHistory, text)
	if len(m.inputHistory) > 200 {
		m.inputHistory = append([]string(nil), m.inputHistory[len(m.inputHistory)-200:]...)
	}
}

func (m *uiModel) resetHistoryCycle() {
	m.historyCycle = false
	m.historyIdx = -1
}

func (m *uiModel) cycleInputHistoryBackward() {
	if len(m.inputHistory) == 0 {
		return
	}
	if !m.historyCycle {
		m.historyCycle = true
		m.historyIdx = len(m.inputHistory) - 1
	} else {
		m.historyIdx--
		if m.historyIdx < 0 {
			m.historyIdx = len(m.inputHistory) - 1
		}
	}
	value := m.inputHistory[m.historyIdx]
	m.input.SetValue(value)
	m.input.SetCursor(len(value))
}

func (m *uiModel) cycleInputHistoryForward() {
	if len(m.inputHistory) == 0 || !m.historyCycle {
		return
	}
	m.historyIdx++
	if m.historyIdx >= len(m.inputHistory) {
		m.historyIdx = 0
	}
	value := m.inputHistory[m.historyIdx]
	m.input.SetValue(value)
	m.input.SetCursor(len(value))
}
