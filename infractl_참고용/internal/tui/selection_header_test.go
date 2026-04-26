package tui

import (
	"strings"
	"testing"
)

func TestSelectionViewOmitsDefaultHeaderWhenEmpty(t *testing.T) {
	s := selectionState{
		active:   true,
		question: "What next?",
		options: []SelectOption{
			{Label: "Option A"},
			{Label: "Option B"},
		},
	}

	rendered := stripANSI(s.renderGeminiBox(80, false))
	if strings.Contains(rendered, "Answer Questions") {
		t.Fatalf("expected empty header to stay hidden, got %q", rendered)
	}
}

func TestSelectionViewShowsExplicitHeader(t *testing.T) {
	s := selectionState{
		active:      true,
		question:    "What next?",
		headerLabel: "Confirm Action",
		options: []SelectOption{
			{Label: "Continue"},
		},
	}

	rendered := stripANSI(s.renderGeminiBox(80, false))
	if !strings.Contains(rendered, "Confirm Action") {
		t.Fatalf("expected explicit header, got %q", rendered)
	}
}

func TestSelectionTextInputOmitsDefaultHeaderWhenEmpty(t *testing.T) {
	s := selectionState{
		active:      true,
		question:    "What next?",
		customInput: "manual value",
	}

	rendered := stripANSI(s.renderGeminiBox(80, false))
	if strings.Contains(rendered, "Answer Questions") {
		t.Fatalf("expected empty header to stay hidden, got %q", rendered)
	}
}
