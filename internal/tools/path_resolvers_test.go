package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveWorkingDirectoryDefaultsToContextWorkingDir(t *testing.T) {
	root := t.TempDir()
	ctx := Context{WorkingDir: root}

	got, err := ResolveWorkingDirectory(ctx, "")
	if err != nil {
		t.Fatalf("resolve working directory: %v", err)
	}
	want, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs root: %v", err)
	}
	if !samePath(got, want) {
		t.Fatalf("unexpected path: got %q want %q", got, want)
	}
}

func TestResolveWorkingDirectoryResolvesRelativePath(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "subdir")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	ctx := Context{WorkingDir: root}

	got, err := ResolveWorkingDirectory(ctx, "subdir")
	if err != nil {
		t.Fatalf("resolve relative working directory: %v", err)
	}
	if !samePath(got, sub) {
		t.Fatalf("unexpected path: got %q want %q", got, sub)
	}
}

func TestResolveWorkingDirectoryRejectsOutsideRoots(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	ctx := Context{WorkingDir: root}

	_, err := ResolveWorkingDirectory(ctx, outside)
	if err == nil {
		t.Fatal("expected outside roots error")
	}
	if !strings.Contains(err.Error(), "outside allowed roots") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveWorkingDirectoryRejectsFilePath(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "file.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	ctx := Context{WorkingDir: root}

	_, err := ResolveWorkingDirectory(ctx, filePath)
	if err == nil {
		t.Fatal("expected directory validation error")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}
