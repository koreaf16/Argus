package promptinput

import (
	"strings"
)

func Render(inner string, width int) string {
	return RenderTextInput(inner, width)
}

func InputBoxHeight(inputView string) int {
	lines := strings.Count(inputView, "\n") + 1
	return lines + 2
}

// InputCursorColumn returns 1-based ANSI column for textarea caret.
func InputCursorColumn(charOffset int) int {
	if charOffset < 0 {
		charOffset = 0
	}
	return len(promptLineIndicator) + 1 + charOffset
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
