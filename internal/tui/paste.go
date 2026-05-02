package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"unicode/utf8"
)

const (
	pasteBlockMinNewlines = 2
	pasteBlockMinChars    = 1025
)

type pastedText struct {
	ID       int
	Content  string
	NumLines int
}

type pastedTextRef struct {
	id    int
	start int
	end   int
}

type promptSubmission struct {
	Display  string
	Expanded string
}

var pastedTextRefPattern = regexp.MustCompile(`\[Pasted text #(\d+)(?: \+\d+ lines)?\]`)

func pastedTextRefNumLines(text string) int {
	count := 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\r':
			count++
			if i+1 < len(text) && text[i+1] == '\n' {
				i++
			}
		case '\n':
			count++
		}
	}
	return count
}

func formatPastedTextRef(id, numLines int) string {
	if numLines == 0 {
		return fmt.Sprintf("[Pasted text #%d]", id)
	}
	return fmt.Sprintf("[Pasted text #%d +%d lines]", id, numLines)
}

func shouldBlockPastedText(text string) bool {
	return pastedTextRefNumLines(text) >= pasteBlockMinNewlines ||
		utf8.RuneCountInString(text) >= pasteBlockMinChars
}

func parsePastedTextRefs(input string) []pastedTextRef {
	matches := pastedTextRefPattern.FindAllStringSubmatchIndex(input, -1)
	refs := make([]pastedTextRef, 0, len(matches))
	for _, match := range matches {
		if len(match) < 4 || match[2] < 0 || match[3] < 0 {
			continue
		}
		id, err := strconv.Atoi(input[match[2]:match[3]])
		if err != nil || id <= 0 {
			continue
		}
		refs = append(refs, pastedTextRef{
			id:    id,
			start: match[0],
			end:   match[1],
		})
	}
	return refs
}

func expandPastedTextRefs(input string, contents map[int]pastedText) string {
	if input == "" || len(contents) == 0 {
		return input
	}
	refs := parsePastedTextRefs(input)
	for i := len(refs) - 1; i >= 0; i-- {
		ref := refs[i]
		content, ok := contents[ref.id]
		if !ok {
			continue
		}
		input = input[:ref.start] + content.Content + input[ref.end:]
	}
	return input
}

func clonePastedTextMap(src map[int]pastedText) map[int]pastedText {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[int]pastedText, len(src))
	for id, content := range src {
		dst[id] = content
	}
	return dst
}

func cloneReferencedPastes(display string, src map[int]pastedText) map[int]pastedText {
	if display == "" || len(src) == 0 {
		return nil
	}
	refs := parsePastedTextRefs(display)
	if len(refs) == 0 {
		return nil
	}
	dst := make(map[int]pastedText, len(refs))
	for _, ref := range refs {
		if content, ok := src[ref.id]; ok {
			dst[ref.id] = content
		}
	}
	if len(dst) == 0 {
		return nil
	}
	return dst
}

func maxPastedTextID(contents map[int]pastedText) int {
	maxID := 0
	for id, content := range contents {
		if id > maxID {
			maxID = id
		}
		if content.ID > maxID {
			maxID = content.ID
		}
	}
	return maxID
}

func (m *uiModel) nextPastedTextID() int {
	m.pasteSeq++
	return m.pasteSeq
}

func (m *uiModel) insertPastedText(content string) bool {
	if content == "" {
		return false
	}
	m.resetHistoryCycle()
	if shouldBlockPastedText(content) {
		id := m.nextPastedTextID()
		numLines := pastedTextRefNumLines(content)
		if m.activePastes == nil {
			m.activePastes = make(map[int]pastedText)
		}
		m.activePastes[id] = pastedText{
			ID:       id,
			Content:  content,
			NumLines: numLines,
		}
		m.input.InsertString(formatPastedTextRef(id, numLines))
	} else {
		m.input.InsertString(content)
	}
	m.refreshSlashSuggestions()
	m.resize()
	return true
}

func (m *uiModel) buildPromptSubmission(display string) promptSubmission {
	return promptSubmission{
		Display:  display,
		Expanded: expandPastedTextRefs(display, m.activePastes),
	}
}
