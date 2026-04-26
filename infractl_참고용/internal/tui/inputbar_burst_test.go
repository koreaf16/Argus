// Package tui
// File: inputbar_burst_test.go
// Description: time-based paste burst detection tests for inputBar
// Responsibility: verify KeyEnter is absorbed during burst and triggers submit otherwise
package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// sendKey is a convenience helper that calls Update and collects any SubmitMsg
// returned via the Cmd (by running it).
func sendKey(ib inputBar, msg tea.KeyMsg) (inputBar, *SubmitMsg) {
	next, cmd := ib.Update(msg)
	if cmd == nil {
		return next, nil
	}
	result := cmd()
	if s, ok := result.(SubmitMsg); ok {
		return next, &s
	}
	return next, nil
}

func keyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func keyEnter() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyEnter}
}

// TestInputBar_BurstEnterDoesNotSubmit verifies that a KeyEnter arriving while
// pendingPasteChunks is non-empty (sticky burst) is absorbed as "\n" and does
// not produce a SubmitMsg.
func TestInputBar_BurstEnterDoesNotSubmit(t *testing.T) {
	ib := newInputBar(80)

	// Seed the pending buffer directly to simulate a burst already in progress.
	ib.pendingPasteChunks = []string{"hello"}
	ib.pendingPasteSeq = 1
	ib.lastKeyTime = time.Now()

	// KeyEnter should be absorbed into the buffer, not submitted.
	next, submit := sendKey(ib, keyEnter())
	if submit != nil {
		t.Fatal("expected no SubmitMsg during paste burst, but got one")
	}

	// The "\n" chunk must have been appended.
	found := false
	for _, chunk := range next.pendingPasteChunks {
		if strings.Contains(chunk, "\n") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected \\n in pendingPasteChunks, got %v", next.pendingPasteChunks)
	}
}

// TestInputBar_NormalEnterSubmits verifies that when there is no active burst
// (lastKeyTime is old enough) a KeyEnter on non-empty input produces a SubmitMsg.
func TestInputBar_NormalEnterSubmits(t *testing.T) {
	ib := newInputBar(80)
	// Type some text directly into the textarea.
	ib.ti.InsertString("hello")
	// lastKeyTime far in the past → not a burst.
	ib.lastKeyTime = time.Now().Add(-500 * time.Millisecond)

	_, submit := sendKey(ib, keyEnter())
	if submit == nil {
		t.Fatal("expected SubmitMsg on Enter with no active burst, but got nil")
	}
	if !strings.Contains(submit.ExpandedInput, "hello") {
		t.Errorf("ExpandedInput %q does not contain expected text", submit.ExpandedInput)
	}
}

// TestInputBar_BurstWindowAbsorbsRapidKeys verifies that KeyRunes arriving
// within pasteBurstWindow after the previous key are routed to handlePasteKeyMsg
// (i.e. they go into pendingPasteChunks) instead of directly into the textarea.
func TestInputBar_BurstWindowAbsorbsRapidKeys(t *testing.T) {
	ib := newInputBar(80)
	// Set lastKeyTime to "just now" to simulate a rapid previous key.
	ib.lastKeyTime = time.Now()

	// The next KeyRunes arrives within the burst window.
	next, submit := sendKey(ib, keyRunes("world"))
	if submit != nil {
		t.Fatal("unexpected SubmitMsg during burst")
	}

	// The chunk should be in the pending buffer, not the textarea.
	found := false
	for _, chunk := range next.pendingPasteChunks {
		if strings.Contains(chunk, "world") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'world' in pendingPasteChunks, got %v; textarea=%q",
			next.pendingPasteChunks, next.ti.Value())
	}
}

// TestInputBar_SpecialKeyBreaksBurst verifies that a non-character key (e.g.
// Ctrl+C) during a burst flushes the pending buffer and does not absorb the key.
func TestInputBar_SpecialKeyBreaksBurst(t *testing.T) {
	ib := newInputBar(80)
	ib.pendingPasteChunks = []string{"some paste"}
	ib.pendingPasteSeq = 1
	ib.lastKeyTime = time.Now()

	// Ctrl+C must break the burst and flush.
	ctrlC := tea.KeyMsg{Type: tea.KeyCtrlC}
	next, _ := sendKey(ib, ctrlC)

	if len(next.pendingPasteChunks) != 0 {
		t.Errorf("expected pendingPasteChunks to be flushed after Ctrl+C, got %v", next.pendingPasteChunks)
	}
}
