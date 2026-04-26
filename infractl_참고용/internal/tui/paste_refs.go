// Package tui
// File: paste_refs.go
// Description: prompt pasted-text placeholder formatting and expansion helpers
// Responsibility: keep long pasted text compact in the input while expanding it on submit
package tui

import (
	"fmt"
	"regexp"

	"github.com/yourorg/infractl/internal/store"
)

const (
	pasteThreshold       = 800
	truncationThreshold  = 10000
	truncationPreviewLen = 1000
)

var referencePattern = regexp.MustCompile(`\[(Pasted text|Image|\.\.\.Truncated text) #(\d+)(?: \+\d+ lines)?(?:\.\.\.)?\]`)

type inputReference struct {
	ID    int
	Match string
	Index int
}

type truncatedInputResult struct {
	DisplayInput   string
	PastedContents map[int]store.PastedContent
}

func getPastedTextRefNumLines(text string) int {
	count := 0
	for _, r := range text {
		if r == '\n' {
			count++
		}
	}
	return count
}

func formatPastedTextRef(id int, numLines int) string {
	if numLines == 0 {
		return fmt.Sprintf("[Pasted text #%d]", id)
	}
	return fmt.Sprintf("[Pasted text #%d +%d lines]", id, numLines)
}

func formatTruncatedTextRef(id int, numLines int) string {
	return fmt.Sprintf("[...Truncated text #%d +%d lines...]", id, numLines)
}

func parseReferences(input string) []inputReference {
	matches := referencePattern.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return nil
	}

	refs := make([]inputReference, 0, len(matches))
	for _, match := range matches {
		if len(match) < 6 {
			continue
		}

		id := 0
		for _, r := range input[match[4]:match[5]] {
			id = id*10 + int(r-'0')
		}
		if id <= 0 {
			continue
		}

		refs = append(refs, inputReference{
			ID:    id,
			Match: input[match[0]:match[1]],
			Index: match[0],
		})
	}
	return refs
}

func expandPastedTextRefs(
	input string,
	pastedContents map[int]store.PastedContent,
) string {
	refs := parseReferences(input)
	expanded := input

	for i := len(refs) - 1; i >= 0; i-- {
		ref := refs[i]
		content, ok := pastedContents[ref.ID]
		if !ok || content.Type != "text" {
			continue
		}
		expanded = expanded[:ref.Index] + content.Content + expanded[ref.Index+len(ref.Match):]
	}
	return expanded
}

func maybeTruncateDisplayInput(
	input string,
	pastedContents map[int]store.PastedContent,
	nextPasteID int,
) truncatedInputResult {
	if len(input) <= truncationThreshold {
		return truncatedInputResult{
			DisplayInput:   input,
			PastedContents: clonePastedContents(pastedContents),
		}
	}

	startLength := truncationPreviewLen / 2
	endLength := truncationPreviewLen / 2
	if startLength > len(input) {
		startLength = len(input)
	}
	if endLength > len(input)-startLength {
		endLength = len(input) - startLength
	}

	startText := input[:startLength]
	endText := input[len(input)-endLength:]
	placeholderContent := input[startLength : len(input)-endLength]

	truncated := clonePastedContents(pastedContents)
	if truncated == nil {
		truncated = map[int]store.PastedContent{}
	}
	truncated[nextPasteID] = store.PastedContent{
		ID:      nextPasteID,
		Type:    "text",
		Content: placeholderContent,
	}

	return truncatedInputResult{
		DisplayInput:   startText + formatTruncatedTextRef(nextPasteID, getPastedTextRefNumLines(placeholderContent)) + endText,
		PastedContents: truncated,
	}
}

func clonePastedContents(contents map[int]store.PastedContent) map[int]store.PastedContent {
	if len(contents) == 0 {
		return nil
	}

	cloned := make(map[int]store.PastedContent, len(contents))
	for id, content := range contents {
		cloned[id] = content
	}
	return cloned
}
