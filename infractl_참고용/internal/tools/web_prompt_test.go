package tools

import (
	"strings"
	"testing"
)

func TestWebSearchToolModelPromptIncludesSourcesGuidance(t *testing.T) {
	prompt := (&WebSearchTool{}).ModelPrompt()
	for _, want := range []string{
		"official docs",
		"web_fetch",
		"Sources",
		"allowed_domains",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected model prompt to contain %q, got %q", want, prompt)
		}
	}
}

func TestWebFetchToolModelPromptGuidesFollowUpFetches(t *testing.T) {
	prompt := (&WebFetchTool{}).ModelPrompt()
	for _, want := range []string{
		"specific URL",
		"url and prompt",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected model prompt to contain %q, got %q", want, prompt)
		}
	}
}
