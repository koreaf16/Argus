// Package tui
// File: inputbar_history.go
// Description: inputbar history and search helpers
// Responsibility: restore prompt drafts and pasted-content state from history entries
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yourorg/infractl/internal/store"
)

func (ib *inputBar) LoadHistory(entries []store.PromptHistoryEntry) {
	ib.history = make([]inputHistoryEntry, 0, len(entries))
	for _, entry := range entries {
		ib.history = append(ib.history, inputHistoryEntry{
			DisplayInput:   entry.Input,
			PastedContents: clonePastedContents(entry.PastedContents),
		})
	}
	ib.historyIdx = -1
}

func (ib *inputBar) handleSearchInput(msg tea.KeyMsg) bool {
	if !ib.searchMode {
		return false
	}

	switch msg.Type {
	case tea.KeyBackspace:
		if len(ib.searchQuery) > 0 {
			ib.searchQuery = ib.searchQuery[:len(ib.searchQuery)-1]
			ib.searchIdx = 0
			ib.applySearch()
		}
		ib.clearSlashSuggestions()
		return true
	case tea.KeyRunes:
		ib.searchQuery += string(msg.Runes)
		ib.searchIdx = 0
		ib.applySearch()
		ib.clearSlashSuggestions()
		return true
	default:
		return false
	}
}

func (ib *inputBar) advanceHistorySearch() {
	if !ib.searchMode {
		ib.searchMode = true
		ib.searchQuery = ""
		ib.searchIdx = 0
		ib.clearSlashSuggestions()
		return
	}

	ib.searchIdx++
	ib.applySearch()
}

func (ib *inputBar) restorePreviousHistory() {
	if ib.historyIdx < len(ib.history)-1 {
		ib.historyIdx++
	}
	ib.restoreHistoryEntry(ib.history[ib.historyIdx])
}

func (ib *inputBar) restoreHistoryEntry(entry inputHistoryEntry) {
	ib.ti.SetValue(entry.DisplayInput)
	ib.ti.CursorEnd()
	ib.pastedContents = clonePastedContents(entry.PastedContents)
	ib.nextPasteID = nextPasteID(ib.pastedContents)
	ib.refreshSlashSuggestions()
}

func (ib *inputBar) restoreNextHistory() bool {
	if ib.historyIdx > 0 {
		ib.historyIdx--
		ib.restoreHistoryEntry(ib.history[ib.historyIdx])
		return true
	}

	if ib.historyIdx == 0 {
		ib.historyIdx = -1
		ib.ti.SetValue("")
		ib.pastedContents = nil
		ib.nextPasteID = 1
		ib.clearSlashSuggestions()
		return true
	}

	return false
}

func (ib *inputBar) applySearch() {
	if ib.searchQuery == "" {
		return
	}

	count := 0
	query := strings.ToLower(ib.searchQuery)
	for _, entry := range ib.history {
		if strings.Contains(strings.ToLower(entry.DisplayInput), query) {
			if count == ib.searchIdx {
				ib.restoreHistoryEntry(entry)
				return
			}
			count++
		}
	}

	ib.searchIdx = 0
}

func (ib *inputBar) handleTabComplete() {
	if ib.hasSlashSuggestions() {
		ib.acceptSlashSuggestion()
		return
	}

	val := ib.ti.Value()
	// 경로(/home/... 등)는 자동완성 대상에서 제외
	if !IsSlashCommandPrefix(val) {
		return
	}
	matches := slashCommandMatchesPrefix(val)
	if len(matches) == 0 {
		return
	}
	ib.ti.SetValue(slashCommandDisplayName(matches[0]))
	ib.ti.CursorEnd()
	ib.clearSlashSuggestions()
}

func (ib *inputBar) shouldRecallHistory() bool {
	return ib.historyIdx >= 0 || (ib.ti.Value() == "" && len(ib.pastedContents) == 0)
}

func nextPasteID(contents map[int]store.PastedContent) int {
	nextID := 1
	for id := range contents {
		if id >= nextID {
			nextID = id + 1
		}
	}
	return nextID
}
