// Package tui
// File: inputbar.go
// Description: bottom-fixed prompt input state and rendering
// Responsibility: coordinate textarea input, prompt draft state, and submission
package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wrap"
	"github.com/yourorg/infractl/internal/store"
)

// SubmitMsg is emitted when the user submits the current prompt draft.
type SubmitMsg struct {
	DisplayInput   string
	ExpandedInput  string
	PastedContents map[int]store.PastedContent
}

type inputHistoryEntry struct {
	DisplayInput   string
	PastedContents map[int]store.PastedContent
}

type inputBar struct {
	ti           textarea.Model
	history      []inputHistoryEntry
	historyIdx   int
	width        int
	maxHeight    int
	historyStore store.HistoryStore
	sessionID    int64
	searchMode   bool
	searchQuery  string
	searchIdx    int
	slashMenu    slashMenuState

	pastedContents     map[int]store.PastedContent
	nextPasteID        int
	pendingPasteChunks []string
	pendingPasteSeq    int
	lastKeyTime        time.Time

	// 펼쳐진(expanded) 붙여넣기 블록 ID 집합 — Space 토글 추적용
	expandedPasteIDs map[int]bool

	// 유휴(Idle) 입력 모드 (비밀번호 프롬프트 등)
	idleMode   bool
	idlePrompt string
	isPassword bool
	replyCh    chan string
	abortCh    chan bool
}

func newInputBar(width int) inputBar {
	ti := textarea.New()
	ti.Placeholder = ""
	ti.CharLimit = 0
	ti.ShowLineNumbers = false
	ti.Prompt = ""
	ti.KeyMap.InsertNewline.SetEnabled(false)
	ti.KeyMap.Paste.SetEnabled(false)
	ti.SetWidth(max(width-5, 1))
	ti.SetHeight(1)
	ti.MaxHeight = 1
	_ = ti.Focus()

	focused, blurred := textarea.DefaultStyles()
	focused.Base = lipgloss.NewStyle()
	focused.CursorLine = lipgloss.NewStyle()
	focused.CursorLineNumber = lipgloss.NewStyle()
	focused.EndOfBuffer = lipgloss.NewStyle()
	focused.LineNumber = lipgloss.NewStyle()
	focused.Prompt = lipgloss.NewStyle()
	focused.Text = lipgloss.NewStyle()
	focused.Placeholder = lipgloss.NewStyle().Foreground(ColorDim)
	blurred = focused
	ti.FocusedStyle = focused
	ti.BlurredStyle = blurred

	ib := inputBar{
		ti:          ti,
		historyIdx:  -1,
		width:       width,
		maxHeight:   1,
		nextPasteID: 1,
	}
	ib.syncHeight()
	return ib
}

func (ib *inputBar) Init() tea.Cmd {
	return nil
}

func (ib *inputBar) setWidth(w int) {
	ib.width = w
	ib.ti.SetWidth(max(w-5, 1))
	ib.syncHeight()
}

// SetBusy는 바쁜 상태일 때 placeholder를 "Enter to queue..." 로 변경한다.
func (ib *inputBar) SetBusy(busy bool) {
	if busy {
		ib.ti.Placeholder = "Enter to queue next prompt..."
	} else {
		ib.ti.Placeholder = ""
	}
}

func (ib *inputBar) setMaxHeight(h int) {
	ib.maxHeight = max(h, 1)
	ib.ti.MaxHeight = ib.maxHeight
	ib.syncHeight()
}

func (ib *inputBar) Value() string {
	return ib.ti.Value()
}

func (ib *inputBar) ExpandedValue() string {
	return expandPastedTextRefs(ib.ti.Value(), ib.pastedContents)
}

func (ib *inputBar) Draft() inputHistoryEntry {
	return inputHistoryEntry{
		DisplayInput:   ib.Value(),
		PastedContents: clonePastedContents(ib.pastedContents),
	}
}

func (ib *inputBar) Reset() {
	ib.ti.Reset()
	ib.historyIdx = -1
	ib.searchMode = false
	ib.searchQuery = ""
	ib.searchIdx = 0
	ib.clearSlashSuggestions()
	ib.pendingPasteChunks = nil
	ib.pendingPasteSeq = 0
	ib.pastedContents = nil
	ib.nextPasteID = 1
	ib.expandedPasteIDs = nil
	ib.lastKeyTime = time.Time{}
	ib.syncHeight()
}

func (ib *inputBar) SetHistoryStore(s store.HistoryStore, sessionID int64) {
	ib.historyStore = s
	ib.sessionID = sessionID
}

func (ib *inputBar) Update(msg tea.Msg) (inputBar, tea.Cmd) {
	switch m := msg.(type) {
	case pasteFlushMsg:
		if m.Seq == ib.pendingPasteSeq {
			ib.flushPendingPaste()
			ib.syncHeight()
		}
		return *ib, nil

	case tea.KeyMsg:
		if isClipboardPasteKey(m) {
			if len(ib.pendingPasteChunks) > 0 {
				ib.flushPendingPaste()
			}
			ib.lastKeyTime = time.Now()
			return ib.handleClipboardPaste()
		}

		now := time.Now()
		inBurst := len(ib.pendingPasteChunks) > 0 ||
			(!ib.lastKeyTime.IsZero() && now.Sub(ib.lastKeyTime) < pasteBurstWindow)
		ib.lastKeyTime = now

		// During a paste burst, absorb character keys into the paste buffer.
		// Only KeyRunes and KeyEnter are absorbed; special keys (Ctrl+C, ESC …)
		// break the burst so they are handled normally by the app.
		if inBurst {
			switch {
			case m.Type == tea.KeyRunes:
				return ib.handlePasteKeyMsg(m)
			case m.Type == tea.KeyEnter && !m.Alt:
				// Convert the Enter into a newline chunk rather than a submit.
				return ib.appendPasteChunk("\n")
			default:
				// Special key: flush pending paste then fall through to normal handling.
				ib.flushPendingPaste()
			}
		}

		if len(ib.pendingPasteChunks) > 0 && !isPasteLikeKeyMsg(m) {
			ib.flushPendingPaste()
		}

		if isPasteLikeKeyMsg(m) {
			return ib.handlePasteKeyMsg(m)
		}

		// Space: 붙여넣기 블록 위에서 접기/펼치기 토글
		if ib.handleSlashSuggestionKey(m) {
			ib.syncHeight()
			return *ib, nil
		}

		if !ib.searchMode && m.Type == tea.KeyRunes && string(m.Runes) == " " {
			if ib.togglePasteAtCursor() {
				ib.syncHeight()
				return *ib, nil
			}
		}

		switch m.Type {
		case tea.KeyCtrlR:
			ib.advanceHistorySearch()
			ib.syncHeight()
			return *ib, nil

		case tea.KeyEsc:
			if ib.searchMode {
				ib.searchMode = false
				ib.searchQuery = ""
				ib.syncHeight()
				return *ib, nil
			}

		case tea.KeyEnter:
			if m.Alt {
				// Alt+Enter: 멀티라인 줄 바꿈 삽입
				ib.ti.InsertString("\n")
				ib.refreshSlashSuggestions()
				ib.syncHeight()
				return *ib, nil
			}
			if ib.searchMode {
				ib.searchMode = false
				ib.searchQuery = ""
				ib.syncHeight()
				return *ib, nil
			}
			draft := ib.Draft()
			if strings.TrimSpace(draft.DisplayInput) == "" {
				return *ib, nil
			}
			ib.AddToHistory(draft)
			submit := SubmitMsg{
				DisplayInput:   draft.DisplayInput,
				ExpandedInput:  expandPastedTextRefs(draft.DisplayInput, draft.PastedContents),
				PastedContents: draft.PastedContents,
			}
			ib.Reset()
			return *ib, func() tea.Msg { return submit }

		case tea.KeyUp:
			if ib.shouldRecallHistory() && len(ib.history) > 0 {
				ib.restorePreviousHistory()
				ib.syncHeight()
				return *ib, nil
			}

		case tea.KeyDown:
			if ib.restoreNextHistory() {
				ib.syncHeight()
				return *ib, nil
			}

		case tea.KeyTab:
			ib.handleTabComplete()
			ib.syncHeight()
			return *ib, nil
		}

		if ib.handleSearchInput(m) {
			ib.syncHeight()
			return *ib, nil
		}
	}

	var cmd tea.Cmd
	ib.ti, cmd = ib.ti.Update(msg)
	ib.cleanupOrphanedPastedContents()
	ib.maybeTruncateCurrentInput()
	ib.refreshSlashSuggestions()
	ib.syncHeight()
	return *ib, cmd
}

func (ib *inputBar) AddToHistory(entry inputHistoryEntry) {
	if strings.TrimSpace(entry.DisplayInput) == "" {
		return
	}

	cloned := inputHistoryEntry{
		DisplayInput:   entry.DisplayInput,
		PastedContents: clonePastedContents(entry.PastedContents),
	}
	ib.history = append([]inputHistoryEntry{cloned}, ib.history...)
	if len(ib.history) > 200 {
		ib.history = ib.history[:200]
	}

	if ib.historyStore != nil {
		go func(entry inputHistoryEntry) {
			_ = ib.historyStore.SavePromptDraft(
				context.Background(),
				entry.DisplayInput,
				entry.PastedContents,
				ib.sessionID,
			)
		}(cloned)
	}
}

func (ib *inputBar) View() string {
	body := ib.ti.View()
	if menu := ib.renderSlashSuggestions(); menu != "" {
		body += "\n" + menu
	}
	if ib.searchMode {
		label := "?뵇 ?????(" + ib.searchQuery + "): "
		return prefixInputLines(
			StyleInputPrompt.Render(label),
			strings.Repeat(" ", lipgloss.Width(label)),
			body,
		)
	}

	return prefixInputLines(StyleInputPrompt.Render("> "), "  ", body)
}

func (ib *inputBar) syncHeight() {
	height := wrappedLineCount(ib.ti.Value(), ib.ti.Width())
	if height < 1 {
		height = 1
	}
	if ib.maxHeight > 0 && height > ib.maxHeight {
		height = ib.maxHeight
	}
	ib.ti.SetHeight(height)
}

func wrappedLineCount(text string, width int) int {
	if width <= 0 || text == "" {
		return 1
	}

	lines := strings.Split(text, "\n")
	total := 0
	for _, line := range lines {
		if line == "" {
			total++
			continue
		}
		wrapped := wrap.String(line, width)
		total += strings.Count(wrapped, "\n") + 1
	}
	if total == 0 {
		return 1
	}
	return total
}

func prefixInputLines(firstPrefix, restPrefix, body string) string {
	lines := strings.Split(body, "\n")
	if len(lines) == 0 {
		return firstPrefix
	}

	var sb strings.Builder
	for i, line := range lines {
		if i > 0 {
			sb.WriteByte('\n')
		}
		if i == 0 {
			sb.WriteString(firstPrefix)
		} else {
			sb.WriteString(restPrefix)
		}
		sb.WriteString(line)
	}
	return sb.String()
}
