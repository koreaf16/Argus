package workspace

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestExecLocalUsesWorkingDir(t *testing.T) {
	mgr := NewManager(NewRegistry(""), nil)
	workDir := t.TempDir()

	cmd, shell := cwdCommandForTest()
	res, err := mgr.execLocal(context.Background(), cmd, ExecOptions{
		WorkingDir: workDir,
		Shell:      shell,
	})
	if err != nil {
		t.Fatalf("execLocal: %v", err)
	}
	if res.Code != 0 {
		t.Fatalf("exit code = %d, stderr = %q, stdout = %q", res.Code, res.Stderr, res.Stdout)
	}
	if !samePathForTest(res.CWD, workDir) {
		t.Fatalf("result cwd = %q, want %q", res.CWD, workDir)
	}
	if !strings.Contains(normalizePathForTest(res.Stdout), normalizePathForTest(workDir)) {
		t.Fatalf("stdout = %q, want cwd %q", res.Stdout, workDir)
	}
}

func TestStartLocalExecStreamingReportsWorkingDir(t *testing.T) {
	mgr := NewManager(NewRegistry(""), nil)
	workDir := t.TempDir()

	cmd, shell := cwdCommandForTest()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	handle, err := mgr.startLocalExecStreaming(ctx, cmd, ExecOptions{
		WorkingDir: workDir,
		Shell:      shell,
	})
	if err != nil {
		t.Fatalf("startLocalExecStreaming: %v", err)
	}

	var output strings.Builder
	for chunk := range handle.Stream {
		output.WriteString(chunk)
	}
	res := <-handle.Result

	if res.Code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", res.Code, res.Stderr)
	}
	if !samePathForTest(res.CWD, workDir) {
		t.Fatalf("result cwd = %q, want %q", res.CWD, workDir)
	}
	if !strings.Contains(normalizePathForTest(output.String()), normalizePathForTest(workDir)) {
		t.Fatalf("stream output = %q, want cwd %q", output.String(), workDir)
	}
}

func cwdCommandForTest() (command, shell string) {
	if runtime.GOOS == "windows" {
		return "[System.IO.Directory]::GetCurrentDirectory()", "powershell"
	}
	return "pwd", "bash"
}

func samePathForTest(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func normalizePathForTest(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}
