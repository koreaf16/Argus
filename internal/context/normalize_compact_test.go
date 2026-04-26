package context_test

import (
	"fmt"
	"strings"
	"testing"

	ctxpkg "github.com/koreaf16/argus/internal/context"
	"github.com/koreaf16/argus/internal/services/llm"
)

func TestNormalizeToolResultForContext_ShellJSON(t *testing.T) {
	raw := `{"stdout":"\u001b[31mPID USER CMD\u001b[0m\n[oracle@db ~]$","stderr":"","code":0}`
	out := ctxpkg.NormalizeToolResultForContext("bash", raw)

	if !strings.Contains(out, "exit_code: 0") {
		t.Fatalf("expected exit code in normalized output: %q", out)
	}
	if !strings.Contains(out, "PID USER CMD") {
		t.Fatalf("expected stdout content in normalized output: %q", out)
	}
	if strings.Contains(out, `\u001b`) || strings.Contains(out, "[oracle@db ~]$") {
		t.Fatalf("expected ansi/prompt noise removed, got: %q", out)
	}
	if strings.Contains(out, `"stdout"`) {
		t.Fatalf("expected JSON wrapper removed, got: %q", out)
	}
}

func TestRenderForLLM_MicrocompactOldShellResults(t *testing.T) {
	g := ctxpkg.NewGraph()
	g.AppendUser("check process")
	g.AppendAssistant("running checks")
	for i := 1; i <= 3; i++ {
		callID := fmt.Sprintf("c%d", i)
		g.AppendToolUse(callID, "bash", []byte(`{"command":"echo ok"}`))
		g.AppendToolResult(
			callID,
			"bash",
			fmt.Sprintf("stdout line for %d", i),
			ctxpkg.ProjectionFull,
			false,
			fmt.Sprintf("art-%d", i),
			fmt.Sprintf("/tmp/art-%d.txt", i),
		)
	}

	msgs := ctxpkg.RenderForLLM(g, ctxpkg.NewTokenEstimator(), 200, 200_000, "check process")
	toolResults := map[string]string{}
	for _, msg := range msgs {
		if msg.Role != llm.RoleUser {
			continue
		}
		for _, block := range msg.Content {
			if block.Type == llm.ContentToolResult {
				toolResults[block.ToolUseID] = block.Text
			}
		}
	}
	if len(toolResults) != 3 {
		t.Fatalf("expected 3 tool results, got %d", len(toolResults))
	}
	if !strings.Contains(toolResults["c1"], ctxpkg.OldToolResultClearedPlaceholder) {
		t.Fatalf("expected oldest shell result cleared, got: %q", toolResults["c1"])
	}
	if strings.Contains(toolResults["c2"], ctxpkg.OldToolResultClearedPlaceholder) {
		t.Fatalf("expected second latest shell result retained, got: %q", toolResults["c2"])
	}
	if strings.Contains(toolResults["c3"], ctxpkg.OldToolResultClearedPlaceholder) {
		t.Fatalf("expected latest shell result retained, got: %q", toolResults["c3"])
	}
	if !strings.Contains(toolResults["c1"], "/tmp/art-1.txt") {
		t.Fatalf("expected cleared payload to retain artifact reference, got: %q", toolResults["c1"])
	}
}
