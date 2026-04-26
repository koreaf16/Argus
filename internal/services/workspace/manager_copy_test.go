package workspace

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestManagerCopyFileLocalToLocal(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.bin")
	dst := filepath.Join(dir, "dst.bin")

	payload := bytes.Repeat([]byte("abc123"), 256*1024)
	if err := os.WriteFile(src, payload, 0o644); err != nil {
		t.Fatalf("seed src: %v", err)
	}

	mgr := NewManager(NewRegistry(""), nil)
	if err := mgr.CopyFile(context.Background(), LocalAlias, src, LocalAlias, dst, true); err != nil {
		t.Fatalf("copy failed: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("copied payload mismatch: got=%d want=%d", len(got), len(payload))
	}
}

func TestManagerCopyFileOverwriteFalse(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatalf("seed src: %v", err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatalf("seed dst: %v", err)
	}

	mgr := NewManager(NewRegistry(""), nil)
	err := mgr.CopyFile(context.Background(), LocalAlias, src, LocalAlias, dst, false)
	if err == nil {
		t.Fatal("expected overwrite protection error")
	}
}
