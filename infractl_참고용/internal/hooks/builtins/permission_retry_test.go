package builtins

import "testing"

func TestPermissionRetryHookBuildsSudoRewrite(t *testing.T) {
	hook := PermissionRetryHook{}
	out, matched := hook.Run(RuntimeInput{
		Event: "PostToolUse",
		Tool:  "shell_exec",
		Input: map[string]any{"command": "touch /etc/app.conf"},
		Output: map[string]any{
			"is_error": true,
			"stderr":   "permission pre-check failed: owner=oracle mode=755 Permission denied",
		},
	})
	if !matched {
		t.Fatal("expected hook to match permission denied case")
	}
	if out.Decision != "ask" {
		t.Fatalf("decision = %q, want ask", out.Decision)
	}
	if out.NewInput["become_method"] != "sudo" {
		t.Fatalf("become_method = %v, want sudo", out.NewInput["become_method"])
	}
	if out.NewInput["become_user"] != "oracle" {
		t.Fatalf("become_user = %v, want oracle", out.NewInput["become_user"])
	}
}

func TestPermissionRetryHookSkipsWhenAlreadyEscalated(t *testing.T) {
	hook := PermissionRetryHook{}
	_, matched := hook.Run(RuntimeInput{
		Event:  "PostToolUse",
		Tool:   "shell_exec",
		Input:  map[string]any{"become_method": "sudo"},
		Output: map[string]any{"is_error": true, "stderr": "Permission denied"},
	})
	if matched {
		t.Fatal("expected hook to skip when become_method already set")
	}
}
