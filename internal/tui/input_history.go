package tui

type inputHistoryEntry struct {
	Display string
	Pasted  map[int]pastedText
}

func (m *uiModel) pushInputHistory(text string) {
	if text == "" {
		return
	}
	m.pushInputHistoryEntry(inputHistoryEntry{
		Display: text,
		Pasted:  cloneReferencedPastes(text, m.activePastes),
	})
}

func (m *uiModel) pushInputHistoryEntry(entry inputHistoryEntry) {
	if entry.Display == "" {
		return
	}
	n := len(m.inputHistory)
	if n > 0 && m.inputHistory[n-1].Display == entry.Display {
		return
	}
	entry.Pasted = clonePastedTextMap(entry.Pasted)
	m.inputHistory = append(m.inputHistory, entry)
	if len(m.inputHistory) > 200 {
		m.inputHistory = append([]inputHistoryEntry(nil), m.inputHistory[len(m.inputHistory)-200:]...)
	}
}

func (m *uiModel) resetHistoryCycle() {
	m.historyCycle = false
	m.historyIdx = -1
	m.historyDraft = nil
}

func (m *uiModel) shouldCycleInputHistoryBackwardOnUp() bool {
	if len(m.inputHistory) == 0 {
		return false
	}
	if m.historyCycle || m.input.Value() == "" {
		return true
	}
	return m.input.Line() == 0
}

func (m *uiModel) shouldCycleInputHistoryForwardOnDown() bool {
	return m.historyCycle && len(m.inputHistory) > 0
}

func (m *uiModel) captureHistoryDraft() *inputHistoryEntry {
	display := m.input.Value()
	if display == "" {
		return nil
	}
	draft := inputHistoryEntry{
		Display: display,
		Pasted:  cloneReferencedPastes(display, m.activePastes),
	}
	return &draft
}

func (m *uiModel) applyInputHistoryEntry(entry inputHistoryEntry) {
	m.input.SetValue(entry.Display)
	m.input.SetCursor(len(entry.Display))
	m.activePastes = clonePastedTextMap(entry.Pasted)
	if maxID := maxPastedTextID(m.activePastes); maxID > m.pasteSeq {
		m.pasteSeq = maxID
	}
}

func (m *uiModel) cycleInputHistoryBackward() {
	if len(m.inputHistory) == 0 {
		return
	}
	if !m.historyCycle {
		m.historyCycle = true
		m.historyIdx = len(m.inputHistory) - 1
		m.historyDraft = m.captureHistoryDraft()
	} else if m.historyIdx > 0 {
		m.historyIdx--
	} else {
		return
	}
	m.applyInputHistoryEntry(m.inputHistory[m.historyIdx])
}

func (m *uiModel) cycleInputHistoryForward() {
	if len(m.inputHistory) == 0 || !m.historyCycle {
		return
	}
	if m.historyIdx < len(m.inputHistory)-1 {
		m.historyIdx++
		m.applyInputHistoryEntry(m.inputHistory[m.historyIdx])
		return
	}

	draft := m.historyDraft
	if draft != nil {
		m.applyInputHistoryEntry(*draft)
	} else {
		m.input.SetValue("")
		m.input.SetCursor(0)
		m.activePastes = nil
	}
	m.resetHistoryCycle()
}
