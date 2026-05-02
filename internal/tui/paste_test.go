package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
)

func TestPastedTextRefFormatting(t *testing.T) {
	if got := formatPastedTextRef(1, 0); got != "[Pasted text #1]" {
		t.Fatalf("unexpected single-line ref: %q", got)
	}
	if got := formatPastedTextRef(1, 2); got != "[Pasted text #1 +2 lines]" {
		t.Fatalf("unexpected multiline ref: %q", got)
	}
}

func TestShortMultilinePasteStaysLiteral(t *testing.T) {
	m := uiModel{input: textarea.New()}
	if ok := m.insertPastedText("a\nb"); !ok {
		t.Fatalf("expected paste insertion")
	}
	if got := m.input.Value(); got != "a\nb" {
		t.Fatalf("expected literal paste, got %q", got)
	}
	if len(m.activePastes) != 0 {
		t.Fatalf("expected no paste block metadata, got %d entries", len(m.activePastes))
	}
}

func TestLargePasteBecomesBlockAndExpandsOnSubmit(t *testing.T) {
	content := "a\nb\nc"
	m := uiModel{input: textarea.New()}
	if ok := m.insertPastedText(content); !ok {
		t.Fatalf("expected paste insertion")
	}

	display := "[Pasted text #1 +2 lines]"
	if got := m.input.Value(); got != display {
		t.Fatalf("expected paste block %q, got %q", display, got)
	}
	submission := m.buildPromptSubmission(m.input.Value())
	if submission.Display != display {
		t.Fatalf("expected compact display, got %q", submission.Display)
	}
	if submission.Expanded != content {
		t.Fatalf("expected expanded content %q, got %q", content, submission.Expanded)
	}
}

func TestLongSingleLinePasteBecomesBlock(t *testing.T) {
	content := strings.Repeat("x", pasteBlockMinChars)
	m := uiModel{input: textarea.New()}
	if ok := m.insertPastedText(content); !ok {
		t.Fatalf("expected paste insertion")
	}
	if got := m.input.Value(); got != "[Pasted text #1]" {
		t.Fatalf("expected single-line paste block, got %q", got)
	}
	if got := m.buildPromptSubmission(m.input.Value()).Expanded; got != content {
		t.Fatalf("expected expanded long paste, got %d chars", len(got))
	}
}

func TestHistoryRestoresPasteMetadata(t *testing.T) {
	content := "a\nb\nc"
	m := uiModel{input: textarea.New()}
	m.insertPastedText(content)
	display := m.input.Value()
	m.pushInputHistory(display)
	m.input.SetValue("")
	m.activePastes = nil

	m.cycleInputHistoryBackward()
	if got := m.input.Value(); got != display {
		t.Fatalf("expected restored display %q, got %q", display, got)
	}
	if got := m.buildPromptSubmission(m.input.Value()).Expanded; got != content {
		t.Fatalf("expected restored paste expansion %q, got %q", content, got)
	}
}
