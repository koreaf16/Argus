// Package tui
// File: inputbar_paste.go
// Description: inputbar pasted-text detection and inline reference helpers
// Responsibility: accumulate pasted chunks and collapse long pasted text into placeholders
package tui

import (
	"regexp"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/yourorg/infractl/internal/store"
)

const pasteCompletionTimeout = 100 * time.Millisecond
const pasteBurstWindow = 10 * time.Millisecond

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

type pasteFlushMsg struct {
	Seq int
}

func isClipboardPasteKey(msg tea.KeyMsg) bool {
	return msg.String() == "ctrl+v"
}

func isPasteLikeKeyMsg(msg tea.KeyMsg) bool {
	if msg.Paste {
		return true
	}

	input := string(msg.Runes)
	return len(input) > pasteThreshold || strings.ContainsAny(input, "\r\n")
}

func (ib *inputBar) handlePasteKeyMsg(msg tea.KeyMsg) (inputBar, tea.Cmd) {
	raw := string(msg.Runes)
	if ib.searchMode {
		text := normalizePastedText(raw)
		if text != "" {
			ib.searchQuery += text
			ib.searchIdx = 0
			ib.applySearch()
			ib.syncHeight()
		}
		return *ib, nil
	}
	return ib.appendPasteChunk(raw)
}

// appendPasteChunk accumulates a raw string into the pending paste buffer and
// (re)schedules the 100 ms flush timer. Call this for both normal paste chunks
// and synthesised newlines from KeyEnter events that arrive during a burst.
func (ib *inputBar) appendPasteChunk(raw string) (inputBar, tea.Cmd) {
	ib.pendingPasteChunks = append(ib.pendingPasteChunks, raw)
	ib.pendingPasteSeq++
	seq := ib.pendingPasteSeq
	return *ib, tea.Tick(pasteCompletionTimeout, func(time.Time) tea.Msg {
		return pasteFlushMsg{Seq: seq}
	})
}

func (ib *inputBar) handleClipboardPaste() (inputBar, tea.Cmd) {
	text, err := clipboard.ReadAll()
	if err != nil || text == "" {
		return *ib, nil
	}

	if ib.searchMode {
		ib.searchQuery += normalizePastedText(text)
		ib.searchIdx = 0
		ib.applySearch()
		ib.syncHeight()
		return *ib, nil
	}

	ib.applyPastedText(text)
	ib.syncHeight()
	return *ib, nil
}

func (ib *inputBar) flushPendingPaste() {
	if len(ib.pendingPasteChunks) == 0 {
		return
	}

	text := strings.Join(ib.pendingPasteChunks, "")
	ib.pendingPasteChunks = nil
	ib.pendingPasteSeq = 0
	ib.applyPastedText(text)
}

func (ib *inputBar) applyPastedText(raw string) {
	text := normalizePastedText(raw)
	if text == "" {
		return
	}

	numLines := getPastedTextRefNumLines(text)
	if len(text) > pasteThreshold || numLines > ib.maxVisiblePasteLines() {
		pasteID := ib.nextPasteID
		ib.nextPasteID++
		if ib.pastedContents == nil {
			ib.pastedContents = map[int]store.PastedContent{}
		}
		ib.pastedContents[pasteID] = store.PastedContent{
			ID:      pasteID,
			Type:    "text",
			Content: text,
		}
		ib.ti.InsertString(formatPastedTextRef(pasteID, numLines))
	} else {
		ib.ti.InsertString(text)
	}

	ib.cleanupOrphanedPastedContents()
	ib.maybeTruncateCurrentInput()
	ib.refreshSlashSuggestions()
}

func (ib *inputBar) maxVisiblePasteLines() int {
	maxLines := 2
	if ib.maxHeight > 0 && ib.maxHeight < maxLines {
		return ib.maxHeight
	}
	return maxLines
}

func (ib *inputBar) cleanupOrphanedPastedContents() {
	if len(ib.pastedContents) == 0 {
		return
	}

	referenced := make(map[int]struct{})
	for _, ref := range parseReferences(ib.ti.Value()) {
		referenced[ref.ID] = struct{}{}
	}

	if len(referenced) == len(ib.pastedContents) {
		return
	}

	nextContents := make(map[int]store.PastedContent, len(referenced))
	for id, content := range ib.pastedContents {
		if _, ok := referenced[id]; ok {
			nextContents[id] = content
		}
	}

	if len(nextContents) == 0 {
		ib.pastedContents = nil
		ib.nextPasteID = 1
		return
	}

	ib.pastedContents = nextContents
	ib.nextPasteID = nextPasteID(ib.pastedContents)
}

func (ib *inputBar) maybeTruncateCurrentInput() {
	input := ib.ti.Value()
	if len(input) <= truncationThreshold || strings.Contains(input, "[...Truncated text #") {
		return
	}

	pasteID := ib.nextPasteID
	result := maybeTruncateDisplayInput(input, ib.pastedContents, pasteID)
	ib.ti.SetValue(result.DisplayInput)
	ib.ti.CursorEnd()
	ib.pastedContents = result.PastedContents
	ib.nextPasteID = nextPasteID(ib.pastedContents)
	ib.refreshSlashSuggestions()
}

func normalizePastedText(raw string) string {
	text := ansiEscapePattern.ReplaceAllString(raw, "")
	replacer := strings.NewReplacer("\r\n", "\n", "\r", "\n", "\t", "    ")
	return replacer.Replace(text)
}
