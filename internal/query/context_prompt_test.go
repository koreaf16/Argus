package query

import (
	"strings"
	"testing"
)

func TestDefaultSystemPrompt_IncludesParallelToolCallGuidance(t *testing.T) {
	t.Parallel()

	blocks := DefaultSystemPrompt()
	if len(blocks) == 0 {
		t.Fatalf("expected system prompt blocks")
	}
	text := blocks[0].Text
	if !strings.Contains(text, "emit multiple tool calls in one response") {
		t.Fatalf("missing parallel tool call guidance in system prompt")
	}
}

