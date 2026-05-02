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

func TestDefaultSystemPrompt_IncludesGreetingGuidance(t *testing.T) {
	t.Parallel()

	blocks := DefaultSystemPrompt()
	if len(blocks) == 0 {
		t.Fatalf("expected system prompt blocks")
	}
	text := blocks[0].Text
	expected := "For simple greetings, chit-chat, or emotional expressions, respond directly in text without using any tools."
	if !strings.Contains(text, expected) {
		t.Fatalf("missing greeting guidance in system prompt")
	}
}

func TestDefaultSystemPrompt_SplitsDynamicTimeBlock(t *testing.T) {
	t.Parallel()

	blocks := DefaultSystemPrompt()
	if len(blocks) < 2 {
		t.Fatalf("expected static and dynamic system prompt blocks, got %d", len(blocks))
	}
	if strings.Contains(blocks[0].Text, "Current system date and time") {
		t.Fatal("static system prompt block should not contain dynamic time")
	}
	if !strings.Contains(blocks[1].Text, "Current system date and time") {
		t.Fatalf("missing dynamic time block: %+v", blocks[1])
	}
	if blocks[0].CacheControl["type"] != "ephemeral" {
		t.Fatalf("static block should opt into provider cache control: %+v", blocks[0].CacheControl)
	}
}

