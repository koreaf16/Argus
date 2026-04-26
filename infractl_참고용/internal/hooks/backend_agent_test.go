// Package hooks
// File: backend_agent_test.go
// Description: agentBackend 단위 테스트 (Phase D Decision 스키마)
// Responsibility: buildAgentPrompt / parseAgentAnswer / Run 동작 검증

package hooks

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// ─── stub AgentRunner ─────────────────────────────────────────────────────────

type stubRunner struct {
	answer   string
	toolUses int
	err      error
}

func (s *stubRunner) RunForHook(_ context.Context, _ string) (string, int, error) {
	return s.answer, s.toolUses, s.err
}

// ─── buildAgentPrompt ─────────────────────────────────────────────────────────

func TestBuildAgentPrompt_SubstitutesArguments(t *testing.T) {
	def := HookDef{Agent: "이벤트: $ARGUMENTS"}
	input := HookInput{Tool: "shell_exec"}

	prompt, err := buildAgentPrompt(def, input)
	if err != nil {
		t.Fatalf("buildAgentPrompt: %v", err)
	}

	if !strings.Contains(prompt, "shell_exec") {
		t.Errorf("prompt missing tool name in serialized input: %q", prompt)
	}
	if strings.Contains(prompt, "$ARGUMENTS") {
		t.Errorf("$ARGUMENTS placeholder not replaced")
	}
}

func TestBuildAgentPrompt_DefaultTemplate(t *testing.T) {
	def := HookDef{} // Agent 필드 없음
	input := HookInput{Tool: "write_file"}

	prompt, err := buildAgentPrompt(def, input)
	if err != nil {
		t.Fatalf("buildAgentPrompt: %v", err)
	}
	if !strings.Contains(prompt, "write_file") {
		t.Error("default template should embed input JSON containing tool name")
	}
	if !strings.Contains(prompt, "decision") {
		t.Error("default template should request decision/reason JSON response")
	}
}

// ─── parseAgentAnswer ─────────────────────────────────────────────────────────

func TestParseAgentAnswer_JSON_Allow(t *testing.T) {
	out := parseAgentAnswer(`{"decision": "allow", "reason": "safe"}`, 1)
	if !out.IsAllow() {
		t.Errorf("want allow, got %q", out.Decision)
	}
	if out.Reason != "safe" {
		t.Errorf("want reason='safe', got %q", out.Reason)
	}
}

func TestParseAgentAnswer_JSON_Deny(t *testing.T) {
	out := parseAgentAnswer(`some text {"decision": "deny", "reason": "dangerous"} more text`, 0)
	if !out.IsDeny() {
		t.Errorf("want deny, got %q", out.Decision)
	}
	if out.Reason != "dangerous" {
		t.Errorf("want reason='dangerous', got %q", out.Reason)
	}
}

func TestParseAgentAnswer_Heuristic_Deny(t *testing.T) {
	for _, word := range []string{"deny", "block", "reject", "거부", "차단"} {
		out := parseAgentAnswer("이 작업을 "+word+" 합니다.", 0)
		if !out.IsDeny() {
			t.Errorf("heuristic deny word %q: want deny decision, got %q", word, out.Decision)
		}
	}
}

func TestParseAgentAnswer_Heuristic_Allow(t *testing.T) {
	out := parseAgentAnswer("이 작업은 안전합니다.", 0)
	if !out.IsAllow() {
		t.Errorf("heuristic: non-deny answer should be allow, got %q", out.Decision)
	}
}

func TestParseAgentAnswer_ToolUsesWithReason(t *testing.T) {
	out := parseAgentAnswer("파일을 확인했습니다. 이상 없음.", 3)
	if !out.IsAllow() {
		t.Errorf("want allow, got %q", out.Decision)
	}
	if out.Reason == "" {
		t.Error("toolUses>0 should set Reason")
	}
}

// ─── agentBackend.Run ─────────────────────────────────────────────────────────

func TestAgentBackend_NilRunner_ReturnsError(t *testing.T) {
	b := &agentBackend{runner: nil}
	_, err := b.Run(context.Background(), HookDef{}, HookInput{})
	if !errors.Is(err, ErrAgentNotImplemented) {
		t.Errorf("want ErrAgentNotImplemented, got %v", err)
	}
}

func TestAgentBackend_RunnerError_Propagated(t *testing.T) {
	b := &agentBackend{runner: &stubRunner{err: errors.New("llm unavailable")}}
	_, err := b.Run(context.Background(), HookDef{}, HookInput{})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "llm unavailable") {
		t.Errorf("error should contain 'llm unavailable', got %v", err)
	}
}

func TestAgentBackend_RunnerAllow(t *testing.T) {
	stub := &stubRunner{answer: `{"decision":"allow","reason":"ok"}`, toolUses: 1}
	b := &agentBackend{runner: stub}

	out, err := b.Run(context.Background(), HookDef{}, HookInput{Tool: "shell_exec"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !out.IsAllow() {
		t.Errorf("want allow, got %q", out.Decision)
	}
	if out.Reason != "ok" {
		t.Errorf("want reason='ok', got %q", out.Reason)
	}
}

func TestAgentBackend_RunnerDeny(t *testing.T) {
	stub := &stubRunner{answer: `{"decision":"deny","reason":"rm -rf detected"}`, toolUses: 0}
	b := &agentBackend{runner: stub}

	out, err := b.Run(context.Background(), HookDef{}, HookInput{Tool: "shell_exec"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !out.IsDeny() {
		t.Errorf("want deny, got %q", out.Decision)
	}
}
