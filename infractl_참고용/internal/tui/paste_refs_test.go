// Package tui
// File: paste_refs_test.go
// Description: pasted-text placeholder helpers regression tests
// Responsibility: verify compact paste refs, expansion, truncation, and normalization behavior
package tui

import (
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/store"
)

func TestFormatPastedTextRef(t *testing.T) {
	if got := formatPastedTextRef(3, 0); got != "[Pasted text #3]" {
		t.Fatalf("formatPastedTextRef without lines = %q", got)
	}
	if got := formatPastedTextRef(4, 7); got != "[Pasted text #4 +7 lines]" {
		t.Fatalf("formatPastedTextRef with lines = %q", got)
	}
}

func TestExpandPastedTextRefs(t *testing.T) {
	input := "head [Pasted text #1] tail [...Truncated text #2 +1 lines...]"
	contents := map[int]store.PastedContent{
		1: {ID: 1, Type: "text", Content: "alpha"},
		2: {ID: 2, Type: "text", Content: "beta\nomega"},
	}

	got := expandPastedTextRefs(input, contents)
	want := "head alpha tail beta\nomega"
	if got != want {
		t.Fatalf("expandPastedTextRefs = %q, want %q", got, want)
	}
}

func TestExpandPastedTextRefsSkipsMissingContent(t *testing.T) {
	input := "before [Pasted text #9] after"

	got := expandPastedTextRefs(input, nil)
	if got != input {
		t.Fatalf("expandPastedTextRefs missing content = %q, want %q", got, input)
	}
}

func TestMaybeTruncateDisplayInput(t *testing.T) {
	input := strings.Repeat("a", truncationThreshold+25)
	result := maybeTruncateDisplayInput(input, nil, 11)

	if result.DisplayInput == input {
		t.Fatal("expected truncated display input")
	}
	if !strings.Contains(result.DisplayInput, "[...Truncated text #11 +0 lines...]") {
		t.Fatalf("missing truncated placeholder: %q", result.DisplayInput)
	}

	content, ok := result.PastedContents[11]
	if !ok {
		t.Fatal("missing truncated pasted content")
	}
	if content.Type != "text" {
		t.Fatalf("unexpected truncated content type %q", content.Type)
	}

	expanded := expandPastedTextRefs(result.DisplayInput, result.PastedContents)
	if expanded != input {
		t.Fatal("expanded truncated input did not restore original content")
	}
}

func TestNormalizePastedText(t *testing.T) {
	raw := "\x1b[31mred\x1b[0m\r\n\tvalue\rline"
	got := normalizePastedText(raw)
	want := "red\n    value\nline"
	if got != want {
		t.Fatalf("normalizePastedText = %q, want %q", got, want)
	}
}
