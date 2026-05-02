package websearch

import "testing"

func TestWebSearchInteractiveModelPreservesStringMessages(t *testing.T) {
	model := &WebSearchInteractiveModel{}
	model.SetResult(`{"query":"x","durationSeconds":0.1,"results":["No results found. Retry with broader terms or different keywords."]}`)
	if len(model.messages) != 1 {
		t.Fatalf("expected diagnostic message, got %+v", model.messages)
	}
	if model.messages[0] != "No results found. Retry with broader terms or different keywords." {
		t.Fatalf("unexpected message: %q", model.messages[0])
	}
}
