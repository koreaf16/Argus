package todo

import (
	"context"
	"strings"
	"testing"
)

func TestWriteToolParsesMarkdown(t *testing.T) {
	store := NewStore()
	tool := &WriteTool{Tracker: NewTracker(store)}

	markdown := "- [ ] 1. Upload Oracle installer\n- [>] 2: Start installation\n* [x] 3. Verify"

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"markdown_list": markdown,
	}, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out.Content, "Upload Oracle installer") {
		t.Fatalf("rendered todo missing content: %q", out.Content)
	}

	item, ok := store.Get("1")
	if !ok {
		t.Fatal("todo item 1 was not stored")
	}
	if item.Content != "Upload Oracle installer" || item.Status != StatusPending {
		t.Fatalf("Item 1 mismatch: %+v", item)
	}

	item2, ok := store.Get("2")
	if !ok {
		t.Fatal("todo item 2 was not stored")
	}
	if item2.Content != "Start installation" || item2.Status != StatusInProgress {
		t.Fatalf("Item 2 mismatch: %+v", item2)
	}

	item3, ok := store.Get("3")
	if !ok {
		t.Fatal("todo item 3 was not stored")
	}
	if item3.Content != "Verify" || item3.Status != StatusCompleted {
		t.Fatalf("Item 3 mismatch: %+v", item3)
	}
}

func TestWriteToolRejectsEmptyMarkdown(t *testing.T) {
	tool := &WriteTool{Tracker: NewTracker(NewStore())}

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"markdown_list": "    \n",
	}, nil)
	if err == nil {
		t.Fatal("expected empty markdown error")
	}
}
