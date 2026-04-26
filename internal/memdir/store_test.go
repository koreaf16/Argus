package memdir

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreBootstrapAndSession(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := NewStore(dir)
	if err := store.Bootstrap(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	payload := map[string]any{"foo": "bar"}
	if err := store.SaveSession("s1", payload); err != nil {
		t.Fatalf("save session: %v", err)
	}
	var out map[string]any
	if err := store.LoadSession("s1", &out); err != nil {
		t.Fatalf("load session: %v", err)
	}
	if got, _ := out["foo"].(string); got != "bar" {
		t.Fatalf("unexpected value: %v", out["foo"])
	}
}

func TestStoreMemoryAddAndFind(t *testing.T) {
	t.Parallel()
	store := NewStore(t.TempDir())
	if err := store.Bootstrap(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if _, err := store.AddMemory("phase2 note", []string{"phase2"}); err != nil {
		t.Fatalf("add memory: %v", err)
	}
	if _, err := store.AddMemory("other note", []string{"misc"}); err != nil {
		t.Fatalf("add memory: %v", err)
	}
	items, err := store.FindRelevantMemories("phase2", 10)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(items))
	}
}

func TestStoreTodoAndPlanPersistence(t *testing.T) {
	t.Parallel()
	store := NewStore(t.TempDir())
	if err := store.Bootstrap(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	todos := []map[string]any{
		{"content": "a", "status": "pending", "activeForm": "doing a"},
	}
	if err := store.SaveTodos("s2", todos); err != nil {
		t.Fatalf("save todos: %v", err)
	}
	var loaded []map[string]any
	if err := store.LoadTodos("s2", &loaded); err != nil {
		t.Fatalf("load todos: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 todo, got %d", len(loaded))
	}

	if err := store.SavePlan("s2", "plan"); err != nil {
		t.Fatalf("save plan: %v", err)
	}
	plan, err := store.LoadPlan("s2")
	if err != nil {
		t.Fatalf("load plan: %v", err)
	}
	if plan != "plan" {
		t.Fatalf("unexpected plan: %q", plan)
	}
}

func TestWriteJSONAtomicWithRenameFallback(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "fallback.json")

	err := writeJSONAtomicWithRename(path, map[string]any{"ok": true}, func(_, _ string) error {
		return errors.New("rename denied")
	})
	if err != nil {
		t.Fatalf("writeJSONAtomicWithRename should fallback on direct write: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fallback output: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("expected written JSON data")
	}
}
