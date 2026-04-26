package markdown

import (
	"regexp"
	"strings"

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

// WrapToWidth word-wraps plain text to at most width visible characters per line.
func WrapToWidth(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	var cur strings.Builder
	curW := 0
	for _, word := range words {
		ww := runewidth.StringWidth(word)
		if curW == 0 {
			cur.WriteString(word)
			curW = ww
		} else if curW+1+ww <= width {
			cur.WriteByte(' ')
			cur.WriteString(word)
			curW += 1 + ww
		} else {
			lines = append(lines, cur.String())
			cur.Reset()
			cur.WriteString(word)
			curW = ww
		}
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return lines
}
