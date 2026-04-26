package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
)

func TestInputHistoryPushAndCycle(t *testing.T) {
	m := uiModel{input: textarea.New()}
	m.pushInputHistory("first")
	m.pushInputHistory("second")
	m.pushInputHistory("second") // duplicate tail ignored

	if len(m.inputHistory) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(m.inputHistory))
	}

	m.cycleInputHistoryBackward()
	if got := m.input.Value(); got != "second" {
		t.Fatalf("expected second, got %q", got)
	}
	m.cycleInputHistoryBackward()
	if got := m.input.Value(); got != "first" {
		t.Fatalf("expected first, got %q", got)
	}
	m.cycleInputHistoryBackward()
	if got := m.input.Value(); got != "second" {
		t.Fatalf("expected wrap to second, got %q", got)
	}
}
