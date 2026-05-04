package query

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/koreaf16/argus/internal/services/llm"
	tool "github.com/koreaf16/argus/internal/tools"
)

func TestPrepareShellToolCallNormalizesSimpleSudo(t *testing.T) {
	engine := &Engine{}
	call := llm.ToolUseStart{
		ID:    "shell-1",
		Name:  "bash",
		Input: json.RawMessage(`{"command":"sudo systemctl restart nginx","server":"prod-api"}`),
	}

	prepared, blocked, reason := engine.prepareShellToolCall(context.Background(), call, tool.Context{}, 0)
	if blocked {
		t.Fatalf("unexpected block: %s", reason)
	}

	var args map[string]any
	if err := json.Unmarshal(prepared.Input, &args); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(args["command"].(string)); got != "systemctl restart nginx" {
		t.Fatalf("command = %q", got)
	}
	if got := strings.TrimSpace(args["as_user"].(string)); got != "root" {
		t.Fatalf("as_user = %q", got)
	}
	guard, ok := args["_shell_guard"].(map[string]any)
	if !ok {
		t.Fatalf("missing _shell_guard: %#v", args["_shell_guard"])
	}
	if got := guard["decision"]; got != "allow" {
		t.Fatalf("decision = %v", got)
	}
}

func TestPrepareShellToolCallBlocksMixedPrivilegeCommand(t *testing.T) {
	engine := &Engine{}
	call := llm.ToolUseStart{
		ID:    "shell-1",
		Name:  "bash",
		Input: json.RawMessage(`{"command":"sudo systemctl restart nginx && tail /var/log/nginx/error.log","server":"prod-api"}`),
	}

	prepared, blocked, reason := engine.prepareShellToolCall(context.Background(), call, tool.Context{}, 0)
	if !blocked {
		t.Fatalf("expected block")
	}
	if !strings.Contains(reason, "must be split") {
		t.Fatalf("reason = %q", reason)
	}

	var args map[string]any
	if err := json.Unmarshal(prepared.Input, &args); err != nil {
		t.Fatal(err)
	}
	guard := args["_shell_guard"].(map[string]any)
	if got := guard["decision"]; got != "denied" {
		t.Fatalf("decision = %v", got)
	}
}
