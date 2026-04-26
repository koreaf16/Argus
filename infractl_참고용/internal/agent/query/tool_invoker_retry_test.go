package query

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yourorg/infractl/internal/hooks"
	"github.com/yourorg/infractl/internal/llm"
)

func captureRetryHooks(t *testing.T) {
	t.Helper()
	hooks.ResetSnapshot()
	t.Cleanup(hooks.ResetSnapshot)
	path := filepath.Join(t.TempDir(), "hooks.yaml")
	yaml := `
PostToolUse:
  - matcher: "shell_exec"
    hooks:
      - type: builtin
        builtin: "permission_retry"
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write hooks config: %v", err)
	}
	if err := hooks.CaptureSnapshot(path); err != nil {
		t.Fatalf("capture hooks: %v", err)
	}
}

func TestToolInvokerPermissionRetryOneShotSuccess(t *testing.T) {
	captureRetryHooks(t)

	calls := 0
	base := func(_ context.Context, tc llm.ToolCall) (string, bool, string) {
		calls++
		if calls == 1 {
			return "Permission denied: owner=oracle", true, ""
		}
		var args map[string]any
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			t.Fatalf("parse retry args: %v", err)
		}
		if args["become_method"] != "sudo" {
			t.Fatalf("become_method = %v, want sudo", args["become_method"])
		}
		if args["become_user"] != "oracle" {
			t.Fatalf("become_user = %v, want oracle", args["become_user"])
		}
		return "retried-ok", false, ""
	}

	approvalCalls := 0
	ti := NewToolInvoker(hooks.NewRunner(nil), base)
	ti.SetApprovalRequester(func(context.Context, string) (bool, error) {
		approvalCalls++
		return true, nil
	})

	out, isErr, _ := ti.Invoke(context.Background(), makeTC("shell_exec", `{"command":"touch /etc/app.conf"}`))
	if isErr {
		t.Fatalf("expected success after retry, got error output: %q", out)
	}
	if out != "retried-ok" {
		t.Fatalf("output = %q, want retried-ok", out)
	}
	if calls != 2 {
		t.Fatalf("base calls = %d, want 2", calls)
	}
	if approvalCalls != 1 {
		t.Fatalf("approval calls = %d, want 1", approvalCalls)
	}
}

func TestToolInvokerPermissionRetryNoThirdAttempt(t *testing.T) {
	captureRetryHooks(t)

	calls := 0
	base := func(_ context.Context, tc llm.ToolCall) (string, bool, string) {
		calls++
		if calls == 2 {
			var args map[string]any
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				t.Fatalf("parse retry args: %v", err)
			}
			if args["become_method"] != "sudo" {
				t.Fatalf("become_method = %v, want sudo", args["become_method"])
			}
		}
		return "Permission denied", true, ""
	}

	ti := NewToolInvoker(hooks.NewRunner(nil), base)
	ti.SetApprovalRequester(func(context.Context, string) (bool, error) {
		return true, nil
	})

	_, isErr, _ := ti.Invoke(context.Background(), makeTC("shell_exec", `{"command":"touch /etc/app.conf"}`))
	if !isErr {
		t.Fatal("expected error after retry still fails")
	}
	if calls != 2 {
		t.Fatalf("expected exactly 2 attempts, got %d", calls)
	}
}
