package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
)

func newSlashModelForTest() uiModel {
	in := textarea.New()
	return uiModel{input: in}
}

func TestRefreshSlashSuggestions_FilterByPrefix(t *testing.T) {
	m := newSlashModelForTest()
	m.input.SetValue("/se")
	m.refreshSlashSuggestions()

	if len(m.slashMatches) < 2 {
		t.Fatalf("expected at least 2 suggestions for /se, got %d", len(m.slashMatches))
	}
	if m.slashMatches[0].Command != "session" {
		t.Fatalf("expected first suggestion to be session, got %q", m.slashMatches[0].Command)
	}
	if m.slashMatches[1].Command != "server" {
		t.Fatalf("expected second suggestion to be server, got %q", m.slashMatches[1].Command)
	}
}

func TestApplySlashSuggestion_CompletesInput(t *testing.T) {
	m := newSlashModelForTest()
	m.input.SetValue("/sta")
	m.refreshSlashSuggestions()

	ok := m.applySlashSuggestion()
	if !ok {
		t.Fatalf("expected suggestion apply to succeed")
	}
	if got := m.input.Value(); got != "/status " {
		t.Fatalf("expected /status completion, got %q", got)
	}
	if len(m.slashMatches) != 0 {
		t.Fatalf("expected overlay to close after completion, got %d matches", len(m.slashMatches))
	}
}

func TestRefreshSlashSuggestions_HidesWhenArgumentsStart(t *testing.T) {
	m := newSlashModelForTest()
	m.input.SetValue("/status detail")
	m.refreshSlashSuggestions()
	if len(m.slashMatches) != 0 {
		t.Fatalf("expected no suggestions once args are present, got %d", len(m.slashMatches))
	}
}
