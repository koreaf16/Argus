package markdown

import (
	"regexp"
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// VisibleWidth returns the display width of s, stripping ANSI escape sequences.
func VisibleWidth(s string) int {
	return runewidth.StringWidth(StripANSI(s))
}

// StripANSI removes all ANSI escape sequences from s.
func StripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// MaxWordWidth returns the display width of the widest non-breakable word in text.
func MaxWordWidth(text string) int {
	words := strings.Fields(StripANSI(text))
	max := 0
	for _, w := range words {
		if ww := runewidth.StringWidth(w); ww > max {
			max = ww
		}
	}
	return max
}

// WrapStyled wraps s (which may contain ANSI escape sequences) to at most width
// visible characters per line, preserving SGR codes. Uses grapheme-aware
// word-wrap with a hard-wrap fallback for runs that exceed width without a
// breakpoint (e.g., long CJK strings or paths).
func WrapStyled(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	wrapped := xansi.Wrap(s, width, " ")
	var out []string
	for _, line := range strings.Split(wrapped, "\n") {
		if VisibleWidth(line) <= width {
			out = append(out, line)
			continue
		}
		// Single word exceeded width (no breakpoint) → hard-wrap.
		out = append(out, strings.Split(xansi.Hardwrap(line, width, false), "\n")...)
	}
	return out
}

// WrapToWidth word-wraps plain text to at most width visible characters per line.
// Delegates to WrapStyled.
func WrapToWidth(text string, width int) []string {
	return WrapStyled(text, width)
}

// IsBoxDrawingLine reports whether s consists of Unicode box-drawing characters
// (U+2500–U+257F) optionally mixed with cell content (spaces, CJK, ASCII text
// inside the box). The first and last non-space rune of the trimmed line must
// both be box-drawing characters.
//
// This is used to detect tables/figures that the LLM rendered with box-drawing
// glyphs so they can be passed through without word-wrapping or re-styling.
func IsBoxDrawingLine(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" || !containsBoxDrawing(t) {
		return false
	}
	runes := []rune(t)
	return isBoxRune(runes[0]) && isBoxRune(runes[len(runes)-1])
}

func isBoxRune(r rune) bool {
	return r >= 0x2500 && r <= 0x257F
}

func containsBoxDrawing(s string) bool {
	for _, r := range s {
		if isBoxRune(r) {
			return true
		}
	}
	return false
}
