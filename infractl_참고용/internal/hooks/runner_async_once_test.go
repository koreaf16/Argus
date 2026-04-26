package hooks

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func captureTestHooks(t *testing.T, yaml string) {
	t.Helper()
	ResetSnapshot()
	t.Cleanup(ResetSnapshot)
	path := filepath.Join(t.TempDir(), "hooks.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write hooks config: %v", err)
	}
	if err := CaptureSnapshot(path); err != nil {
		t.Fatalf("capture hooks snapshot: %v", err)
	}
}

func TestRunPreToolUseAsyncDoesNotBlockOrDeny(t *testing.T) {
	captureTestHooks(t, `
PreToolUse:
  - matcher: "shell_exec"
    hooks:
      - type: unknown_backend
        async: true
`)

	out := NewRunner(nil).RunPreToolUse(context.Background(), HookInput{Tool: "shell_exec"})
	if !out.IsAllow() {
		t.Fatalf("async PreToolUse hook should allow immediately, got %q", out.Decision)
	}
}

func TestRunPreToolUseOnceSkipsAfterFirstRun(t *testing.T) {
	captureTestHooks(t, `
PreToolUse:
  - matcher: "shell_exec"
    hooks:
      - type: unknown_backend
        once: true
`)

	r := NewRunner(nil)
	first := r.RunPreToolUse(context.Background(), HookInput{Tool: "shell_exec"})
	if !first.IsDeny() {
		t.Fatalf("first once hook should execute and deny on backend error, got %q", first.Decision)
	}
	second := r.RunPreToolUse(context.Background(), HookInput{Tool: "shell_exec"})
	if !second.IsAllow() {
		t.Fatalf("second once hook should be skipped and allow, got %q", second.Decision)
	}
}
