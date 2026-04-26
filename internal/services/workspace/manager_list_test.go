package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestManagerListDirDepth(t *testing.T) {
	root := t.TempDir()
	level1 := filepath.Join(root, "a")
	level2 := filepath.Join(level1, "b")
	if err := os.MkdirAll(level2, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(level1, "one.txt"), []byte("1"), 0o644); err != nil {
		t.Fatalf("write l1 file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(level2, "two.txt"), []byte("2"), 0o644); err != nil {
		t.Fatalf("write l2 file: %v", err)
	}

	mgr := NewManager(NewRegistry(""), nil)
	items, err := mgr.ListDir(context.Background(), LocalAlias, root, true, 0)
	if err != nil {
		t.Fatalf("list dir: %v", err)
	}

	foundL1 := false
	foundL2 := false
	for _, item := range items {
		if item.Path == filepath.Join(level1, "one.txt") {
			foundL1 = true
		}
		if item.Path == filepath.Join(level2, "two.txt") {
			foundL2 = true
		}
	}
	if !foundL1 {
		t.Fatal("expected level1 file in list")
	}
	if !foundL2 {
		t.Fatal("expected nested file in recursive list")
	}
}
