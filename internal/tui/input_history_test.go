package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
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
	if got := m.input.Value(); got != "first" {
		t.Fatalf("expected to stay on oldest entry, got %q", got)
	}
}

func TestUpKeyCyclesHistoryWhileHistoryActive(t *testing.T) {
	m := uiModel{input: textarea.New(), width: 80}
	m.pushInputHistory("first")
	m.pushInputHistory("second")

	m = updateInputHistoryTestModel(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if got := m.input.Value(); got != "second" {
		t.Fatalf("expected latest history entry, got %q", got)
	}
	m = updateInputHistoryTestModel(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if got := m.input.Value(); got != "first" {
		t.Fatalf("expected previous history entry, got %q", got)
	}
}

func TestHistoryDownRestoresDraft(t *testing.T) {
	m := uiModel{input: textarea.New(), width: 80}
	m.pushInputHistory("previous")
	m.input.SetValue("draft")
	m.input.SetCursor(len("draft"))

	m = updateInputHistoryTestModel(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if got := m.input.Value(); got != "previous" {
		t.Fatalf("expected history entry, got %q", got)
	}
	m = updateInputHistoryTestModel(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if got := m.input.Value(); got != "draft" {
		t.Fatalf("expected draft restore, got %q", got)
	}
	if m.historyCycle {
		t.Fatalf("expected history cycle to reset")
	}
}

func TestUpKeyInsideMultilineInputDoesNotCycleHistory(t *testing.T) {
	m := uiModel{input: textarea.New(), width: 80}
	m.input.SetWidth(80)
	m.pushInputHistory("previous")
	m.input.SetValue("line one\nline two")
	m.input.SetCursor(len("line one\nline two"))
	if got := m.input.Line(); got == 0 {
		t.Fatalf("test setup expected cursor below first line")
	}

	m = updateInputHistoryTestModel(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if got := m.input.Value(); got == "previous" {
		t.Fatalf("did not expect history cycle from a multiline middle row")
	}
	if m.historyCycle {
		t.Fatalf("expected history cycle to remain inactive")
	}
}

func updateInputHistoryTestModel(t *testing.T, m uiModel, msg tea.Msg) uiModel {
	t.Helper()
	nextModel, _ := m.Update(msg)
	next, ok := nextModel.(uiModel)
	if !ok {
		t.Fatalf("expected uiModel, got %T", nextModel)
	}
	return next
}
