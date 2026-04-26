package hooks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

func bashCmd(command string) HookCommand {
	return HookCommand{
		Type:    HookCommandTypeCommand,
		Command: command,
		Shell:   "bash",
	}
}

func TestExecutor_exit0_continue(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires bash")
	}
	e := &Executor{}
	res := e.Run(context.Background(), bashCmd("exit 0"), HookInput{})
	if !res.Continue {
		t.Errorf("exit 0 should be Continue=true, got false (stderr: %q)", res.Stderr)
	}
	if res.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", res.ExitCode)
	}
}

func TestExecutor_exit2_blocks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires bash")
	}
	e := &Executor{}
	res := e.Run(context.Background(), bashCmd("echo 'blocked' >&2; exit 2"), HookInput{})
	if res.Continue {
		t.Error("exit 2 should set Continue=false")
	}
	if res.ExitCode != 2 {
		t.Errorf("expected exit code 2, got %d", res.ExitCode)
	}
	if !strings.Contains(res.StopReason, "blocked") {
		t.Errorf("expected stderr in StopReason, got %q", res.StopReason)
	}
}

func TestExecutor_exit1_continues_with_warning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires bash")
	}
	e := &Executor{}
	res := e.Run(context.Background(), bashCmd("echo 'warn' >&2; exit 1"), HookInput{})
	if !res.Continue {
		t.Error("exit 1 should not block (Continue=true)")
	}
	if res.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", res.ExitCode)
	}
}

func TestExecutor_jsonStdout_block(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires bash")
	}
	e := &Executor{}
	res := e.Run(context.Background(), bashCmd(`echo '{"continue":false,"stopReason":"security policy"}'`), HookInput{})
	if res.Continue {
		t.Error("JSON {continue:false} should set Continue=false")
	}
	if res.StopReason != "security policy" {
		t.Errorf("expected stopReason 'security policy', got %q", res.StopReason)
	}
}

func TestExecutor_jsonStdout_decision_block(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires bash")
	}
	e := &Executor{}
	res := e.Run(context.Background(), bashCmd(`echo '{"decision":"block","reason":"forbidden"}'`), HookInput{})
	if res.Continue {
		t.Error("JSON {decision:block} should set Continue=false")
	}
	if res.Decision != "block" {
		t.Errorf("expected Decision=block, got %q", res.Decision)
	}
}

func TestExecutor_envVarsInjected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires bash")
	}
	e := &Executor{}
	input := HookInput{
		SessionID:     "test-session",
		WorkingDir:    "/tmp",
		ToolName:      "Bash",
		ToolInputJSON: `{"command":"ls"}`,
	}
	res := e.Run(context.Background(), bashCmd("echo $ARGUS_SESSION_ID:$ARGUS_TOOL_NAME"), input)
	if !strings.Contains(res.Stdout, "test-session:Bash") {
		t.Errorf("env vars not injected, stdout: %q", res.Stdout)
	}
}

func TestExecutor_httpHook_continue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"continue":true}`))
	}))
	defer srv.Close()

	e := &Executor{}
	res := e.Run(context.Background(), HookCommand{
		Type: HookCommandTypeHTTP,
		URL:  srv.URL,
	}, HookInput{ToolName: "Bash"})

	if !res.Continue {
		t.Errorf("expected Continue=true, got false (stderr: %q)", res.Stderr)
	}
}

func TestExecutor_httpHook_block(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"continue":false,"stopReason":"blocked by http hook"}`))
	}))
	defer srv.Close()

	e := &Executor{}
	res := e.Run(context.Background(), HookCommand{
		Type: HookCommandTypeHTTP,
		URL:  srv.URL,
	}, HookInput{ToolName: "Bash"})

	if res.Continue {
		t.Error("expected Continue=false from http hook")
	}
	if res.StopReason != "blocked by http hook" {
		t.Errorf("unexpected StopReason: %q", res.StopReason)
	}
}

func TestExecutor_httpHook_receivesInput(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	e := &Executor{}
	e.Run(context.Background(), HookCommand{
		Type: HookCommandTypeHTTP,
		URL:  srv.URL,
	}, HookInput{
		SessionID: "sess-123",
		ToolName:  "Bash",
	})

	if received["session_id"] != "sess-123" {
		t.Errorf("expected session_id=sess-123, got %v", received["session_id"])
	}
	if received["tool_name"] != "Bash" {
		t.Errorf("expected tool_name=Bash, got %v", received["tool_name"])
	}
}

func TestExecutor_httpHook_400isWarning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	e := &Executor{}
	res := e.Run(context.Background(), HookCommand{
		Type: HookCommandTypeHTTP,
		URL:  srv.URL,
	}, HookInput{})

	// HTTP 400 은 비차단 경고여야 함
	if !res.Continue {
		t.Error("HTTP 400 should not block (Continue must be true)")
	}
	if res.ExitCode == 0 {
		t.Error("HTTP 400 should set non-zero exit code")
	}
}

func TestInterpolateEnvVars(t *testing.T) {
	t.Setenv("MY_TOKEN", "secret")

	cases := []struct {
		value   string
		allowed []string
		want    string
	}{
		{"Bearer $MY_TOKEN", []string{"MY_TOKEN"}, "Bearer secret"},
		{"Bearer $MY_TOKEN", []string{}, "Bearer "}, // allowed 목록 없으면 차단
		{"Bearer ${MY_TOKEN}", []string{"MY_TOKEN"}, "Bearer secret"},
		{"no-var", []string{"MY_TOKEN"}, "no-var"},
	}
	for _, c := range cases {
		got := interpolateEnvVars(c.value, c.allowed)
		if got != c.want {
			t.Errorf("interpolateEnvVars(%q, %v) = %q, want %q", c.value, c.allowed, got, c.want)
		}
	}
}

func Test_parseJSONOutput(t *testing.T) {
	cases := []struct {
		name         string
		stdout       string
		wantContinue bool
		wantDecision string
		wantReason   string
	}{
		{"no json", "plain text\nmore text", true, "", ""},
		{"continue false", `{"continue":false,"stopReason":"denied"}`, false, "", ""},
		{"decision block", `{"decision":"block","reason":"bad"}`, false, "block", "bad"},
		{"decision approve", `{"decision":"approve"}`, true, "approve", ""},
		{"json after text", "info\n{\"continue\":false}", false, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := &HookExecResult{Continue: true}
			parseJSONOutput(c.stdout, result)
			if result.Continue != c.wantContinue {
				t.Errorf("Continue: got %v, want %v", result.Continue, c.wantContinue)
			}
			if result.Decision != c.wantDecision {
				t.Errorf("Decision: got %q, want %q", result.Decision, c.wantDecision)
			}
		})
	}
}

func TestExecutor_promptHook_nilSubQueryFn_nonBlocking(t *testing.T) {
	e := &Executor{subQueryFn: nil}
	res := e.Run(context.Background(), HookCommand{
		Type:   HookCommandTypePrompt,
		Prompt: "Is this allowed?",
	}, HookInput{ToolName: "Bash"})

	if !res.Continue {
		t.Error("nil subQueryFn should not block execution")
	}
	if res.ExitCode == 0 {
		t.Error("nil subQueryFn should set non-zero exit code (warning)")
	}
}

func TestExecutor_promptHook_allowResponse(t *testing.T) {
	e := &Executor{
		subQueryFn: func(_ context.Context, _, _ string) (string, error) {
			return `{"ok":true}`, nil
		},
	}
	res := e.Run(context.Background(), HookCommand{
		Type:   HookCommandTypePrompt,
		Prompt: "Allow?",
	}, HookInput{ToolName: "Bash"})

	if !res.Continue {
		t.Errorf("ok=true should allow, got Continue=false, StopReason=%q", res.StopReason)
	}
	if res.ExitCode != 0 {
		t.Errorf("ok=true should give exit 0, got %d", res.ExitCode)
	}
}

func TestExecutor_promptHook_blockResponse(t *testing.T) {
	e := &Executor{
		subQueryFn: func(_ context.Context, _, _ string) (string, error) {
			return `{"ok":false,"reason":"policy violation"}`, nil
		},
	}
	res := e.Run(context.Background(), HookCommand{
		Type:   HookCommandTypePrompt,
		Prompt: "Block this?",
	}, HookInput{ToolName: "Bash"})

	if res.Continue {
		t.Error("ok=false should block execution (Continue must be false)")
	}
	if res.ExitCode != 2 {
		t.Errorf("block should give exit 2, got %d", res.ExitCode)
	}
	if !strings.Contains(res.StopReason, "policy violation") {
		t.Errorf("StopReason should contain reason, got %q", res.StopReason)
	}
}

func TestExecutor_promptHook_argumentsSubstitution(t *testing.T) {
	var capturedUser string
	e := &Executor{
		subQueryFn: func(_ context.Context, _, userPrompt string) (string, error) {
			capturedUser = userPrompt
			return `{"ok":true}`, nil
		},
	}
	e.Run(context.Background(), HookCommand{
		Type:   HookCommandTypePrompt,
		Prompt: "Check: $ARGUMENTS",
	}, HookInput{ToolName: "Bash", WorkingDir: "/tmp"})

	if !strings.Contains(capturedUser, "Bash") {
		t.Errorf("$ARGUMENTS should be substituted with HookInput JSON containing ToolName; got %q", capturedUser)
	}
}
