package tui

import "time"

const maxToolHistory = 200

type toolHistoryEntry struct {
	toolID       string
	toolName     string
	target       string
	result       string
	metadataJSON string
	duration     time.Duration
	success      bool
	finishedAt   time.Time
	shellLines   []string
	shellTotal   int
}

type toolHistory struct {
	entries []toolHistoryEntry
}

func (h *toolHistory) Add(e toolHistoryEntry) {
	e.finishedAt = time.Now()
	h.entries = append(h.entries, e)
	if len(h.entries) > maxToolHistory {
		h.entries = h.entries[len(h.entries)-maxToolHistory:]
	}
}

func (h *toolHistory) Len() int {
	return len(h.entries)
}

func (h *toolHistory) Get(i int) (toolHistoryEntry, bool) {
	if i < 0 || i >= len(h.entries) {
		return toolHistoryEntry{}, false
	}
	return h.entries[i], true
}

func (h *toolHistory) LatestShell() (toolHistoryEntry, bool) {
	for i := len(h.entries) - 1; i >= 0; i-- {
		if len(h.entries[i].shellLines) > 0 {
			return h.entries[i], true
		}
	}
	return toolHistoryEntry{}, false
}

func (h *toolHistory) UpdateTaskProgress(toolID string, metadataJSON string) {
	for i := len(h.entries) - 1; i >= 0; i-- {
		if h.entries[i].toolID == toolID {
			h.entries[i].metadataJSON = metadataJSON
			return
		}
	}
}
