package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNormalizeLLMLogTextExtractsSSEContent(t *testing.T) {
	raw := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"\uc548\ub155"}}]}`,
		`data: {"choices":[{"delta":{"content":"\n\uc138\uacc4"}}]}`,
		`data: [DONE]`,
	}, "\n")

	got := normalizeLLMLogText(raw)
	want := "\uc548\ub155\n\uc138\uacc4"
	if got != want {
		t.Fatalf("normalizeLLMLogText() = %q, want %q", got, want)
	}
}

func TestNormalizeLLMLogTextDecodesEscapes(t *testing.T) {
	got := normalizeLLMLogText(`\uc548\ub155\n\ud14c\uc2a4\ud2b8`)
	want := "\uc548\ub155\n\ud14c\uc2a4\ud2b8"
	if got != want {
		t.Fatalf("normalizeLLMLogText() = %q, want %q", got, want)
	}
}

func TestNormalizeLLMLogTextExtractsChatResponseContent(t *testing.T) {
	raw := `{"choices":[{"message":{"content":"\uc548\ub155\n\uc138\uacc4"}}]}`

	got := normalizeLLMLogText(raw)
	want := "\uc548\ub155\n\uc138\uacc4"
	if got != want {
		t.Fatalf("normalizeLLMLogText() = %q, want %q", got, want)
	}
}

func TestLogRequestJSONWritesAllSentContentToLLMLog(t *testing.T) {
	tmp := chdirTemp(t)

	raw := []byte(`{"messages":[` +
		`{"role":"system","content":"system\nprompt"},` +
		`{"role":"user","content":"\uc548\ub155\n\ud14c\uc2a4\ud2b8"},` +
		`{"role":"assistant","content":"previous answer"},` +
		`{"role":"tool","content":"tool result"}` +
		`]}`)
	LogRequestJSON("test-model", raw)

	logText := readLLMLog(t, tmp)
	for _, want := range []string{
		"system\nprompt",
		"\uc548\ub155\n\ud14c\uc2a4\ud2b8",
		"previous answer",
		"tool result",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("llm.log = %q, want it to contain %q", logText, want)
		}
	}
	if strings.Contains(logText, `"content"`) || strings.Contains(logText, `\n`) {
		t.Fatalf("llm.log still contains JSON or escaped newlines: %q", logText)
	}
}

func TestLogResponseJSONWritesOnlyResponseContentToLLMLog(t *testing.T) {
	tmp := chdirTemp(t)

	raw := []byte(`{"choices":[{"message":{"content":"\uc548\ub155\n\uc138\uacc4"}}]}`)
	LogResponseJSON("test-model", raw)

	logText := readLLMLog(t, tmp)
	want := "\uc548\ub155\n\uc138\uacc4"
	if !strings.Contains(logText, want) {
		t.Fatalf("llm.log = %q, want it to contain %q", logText, want)
	}
	if strings.Contains(logText, `"choices"`) || strings.Contains(logText, `"content"`) {
		t.Fatalf("llm.log still contains JSON: %q", logText)
	}
}

func TestParseStreamWithoutThinkTagReturnsContentAndLogsReadableText(t *testing.T) {
	tmp := chdirTemp(t)

	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"\uc548\ub155"}}]}`,
		`data: {"choices":[{"delta":{"content":"\n\uc138\uacc4"}}]}`,
		`data: [DONE]`,
	}, "\n")

	c := &OpenAIClient{}
	resp, err := c.parseStream("test-model", strings.NewReader(stream), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "\uc548\ub155\n\uc138\uacc4"
	if resp.Content != want {
		t.Fatalf("Content = %q, want %q", resp.Content, want)
	}

	logText := readLLMLog(t, tmp)
	if strings.Contains(logText, "data:") || strings.Contains(logText, `"delta"`) {
		t.Fatalf("llm.log contains raw SSE/JSON: %q", logText)
	}
	if !strings.Contains(logText, want) {
		t.Fatalf("llm.log = %q, want it to contain %q", logText, want)
	}
}

func TestSanitizeAssistantContentStripsPlainTextReasoningLeak(t *testing.T) {
	raw := strings.TrimSpace(`
Thinking Process:

1. Analyze the Input:
   - User: "하이?"

2. Draft the response:
   - Keep it short.

Final Output Generation.

안녕하세요!

필요한 작업 말씀해 주세요.
`)

	got := sanitizeAssistantContent(raw)
	want := "안녕하세요!\n\n필요한 작업 말씀해 주세요."
	if got != want {
		t.Fatalf("sanitizeAssistantContent() = %q, want %q", got, want)
	}
}

func TestSanitizeAssistantContentWithOnlyLeakReturnsEmpty(t *testing.T) {
	raw := strings.TrimSpace(`
Thinking Process:

1. Analyze the Input:
   - User: "하이?"
`)

	if got := sanitizeAssistantContent(raw); got != "" {
		t.Fatalf("sanitizeAssistantContent() = %q, want empty string", got)
	}
}

func TestParseStreamSanitizesPlainTextReasoningLeak(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Thinking Process:\n\n"}}]}`,
		`data: {"choices":[{"delta":{"content":"1. Analyze the Input:\n- User: hi\n\n"}}]}`,
		`data: {"choices":[{"delta":{"content":"Final Output Generation.\n\n안녕하세요.\n"}}]}`,
		`data: {"choices":[{"delta":{"content":"필요한 작업 말씀해 주세요."}}]}`,
		`data: [DONE]`,
	}, "\n")

	c := &OpenAIClient{}
	resp, err := c.parseStream("test-model", strings.NewReader(stream), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "안녕하세요.\n필요한 작업 말씀해 주세요."
	if resp.Content != want {
		t.Fatalf("Content = %q, want %q", resp.Content, want)
	}
}

func TestParseStreamDoesNotEmitLeakedReasoningTokens(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Thinking Process:\n\n"}}]}`,
		`data: {"choices":[{"delta":{"content":"1. Analyze the Input:\n- User: hi\n\n"}}]}`,
		`data: {"choices":[{"delta":{"content":"Final Output Generation.\n\n안녕하세요.\n"}}]}`,
		`data: {"choices":[{"delta":{"content":"필요한 작업 말씀해 주세요."}}]}`,
		`data: [DONE]`,
	}, "\n")

	var emitted strings.Builder
	c := &OpenAIClient{}
	resp, err := c.parseStream("test-model", strings.NewReader(stream), nil, func(token string) {
		emitted.WriteString(token)
	})
	if err != nil {
		t.Fatal(err)
	}

	want := "안녕하세요.\n필요한 작업 말씀해 주세요."
	if emitted.String() != want {
		t.Fatalf("streamed content = %q, want %q", emitted.String(), want)
	}
	if resp.Content != want {
		t.Fatalf("Content = %q, want %q", resp.Content, want)
	}
}

func TestParseStreamDoesNotEmitThinkBlockWithoutOpenTag(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"The user is saying \"하이?\" which is a casual greeting in Korean.\n\n"}}]}`,
		`data: {"choices":[{"delta":{"content":"This is a casual greeting, so according to my rules, I should not call any tools.\n\n"}}]}`,
		`data: {"choices":[{"delta":{"content":"I'll respond in a friendly, casual manner in Korean.\n</think>\n"}}]}`,
		`data: {"choices":[{"delta":{"content":"안녕하세요.\n필요한 작업 말씀해 주세요."}}]}`,
		`data: [DONE]`,
	}, "\n")

	var emitted strings.Builder
	c := &OpenAIClient{}
	resp, err := c.parseStream("test-model", strings.NewReader(stream), nil, func(token string) {
		emitted.WriteString(token)
	})
	if err != nil {
		t.Fatal(err)
	}

	want := "안녕하세요.\n필요한 작업 말씀해 주세요."
	if emitted.String() != want {
		t.Fatalf("streamed content = %q, want %q", emitted.String(), want)
	}
	if resp.Content != want {
		t.Fatalf("Content = %q, want %q", resp.Content, want)
	}
}

func TestExtractInlineToolCallsParsesJSONFormat(t *testing.T) {
	input := `text before<tool_call>{"name":"server_command","arguments":{"server":"test","command":"ls"}}</tool_call>text after`
	calls, cleaned := extractInlineToolCalls(input)
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	if calls[0].Function.Name != "server_command" {
		t.Errorf("Name = %q, want server_command", calls[0].Function.Name)
	}
	if !strings.Contains(calls[0].Function.Arguments, `"server":"test"`) {
		t.Errorf("Arguments = %q, want it to contain server:test", calls[0].Function.Arguments)
	}
	if strings.Contains(cleaned, "<tool_call>") {
		t.Errorf("cleaned content still has <tool_call> tags: %q", cleaned)
	}
}

func TestExtractInlineToolCallsParsesQwenXMLFormat(t *testing.T) {
	input := "<tool_call>\n<function=server_command>\n<parameter=server>\ntest\n</parameter>\n<parameter=command>\ncat /etc/os-release\n</parameter>\n</function>\n</tool_call>"
	calls, cleaned := extractInlineToolCalls(input)
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1; cleaned=%q", len(calls), cleaned)
	}
	if calls[0].Function.Name != "server_command" {
		t.Errorf("Name = %q, want server_command", calls[0].Function.Name)
	}
	if !strings.Contains(calls[0].Function.Arguments, `"server"`) {
		t.Errorf("Arguments = %q, want it to contain server key", calls[0].Function.Arguments)
	}
	if !strings.Contains(calls[0].Function.Arguments, "test") {
		t.Errorf("Arguments = %q, want it to contain 'test' value", calls[0].Function.Arguments)
	}
	if strings.Contains(cleaned, "<tool_call>") {
		t.Errorf("cleaned content still has <tool_call> tags: %q", cleaned)
	}
}

func TestExtractInlineToolCallsMultipleQwenXMLBlocks(t *testing.T) {
	block1 := "<tool_call>\n<function=cmd_a>\n<parameter=x>1</parameter>\n</function>\n</tool_call>"
	block2 := "<tool_call>\n<function=cmd_b>\n<parameter=y>2</parameter>\n</function>\n</tool_call>"
	calls, _ := extractInlineToolCalls(block1 + "\n" + block2)
	if len(calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(calls))
	}
	if calls[0].Function.Name != "cmd_a" || calls[1].Function.Name != "cmd_b" {
		t.Errorf("names = %q %q, want cmd_a cmd_b", calls[0].Function.Name, calls[1].Function.Name)
	}
}

func TestParseStreamExtractsGemmaInlineToolCallVariant(t *testing.T) {
	_ = chdirTemp(t)

	content := `<|tool_call>call:shell_exec{command:<|"|>find /tmp -name "*INFRACTL_QE_VERIFY*.trc"<|"|>,reason:<|"|>search trace<|"|>,target:<|"|>oracle<|"|>}<tool_call|>`
	payload, err := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{"delta": map[string]any{"content": content}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	stream := "data: " + string(payload) + "\n" + "data: [DONE]\n"

	c := &OpenAIClient{}
	resp, err := c.parseStream("test-model", strings.NewReader(stream), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1; content=%q", len(resp.ToolCalls), resp.Content)
	}
	if resp.ToolCalls[0].Function.Name != "shell_exec" {
		t.Fatalf("tool name = %q, want shell_exec", resp.ToolCalls[0].Function.Name)
	}
	for _, want := range []string{`"target":"oracle"`, "INFRACTL_QE_VERIFY"} {
		if !strings.Contains(resp.ToolCalls[0].Function.Arguments, want) {
			t.Fatalf("arguments = %q, missing %q", resp.ToolCalls[0].Function.Arguments, want)
		}
	}
}

// --- doRequestWith HTTP 상태 체크 테스트 ---

func TestParseStreamExtractsNamespacedGemmaTodoWriteCall(t *testing.T) {
	_ = chdirTemp(t)

	content := `call:infractl:todo_write{todo_list:['1. Inventory check','2. Cleanup namespace']}`
	payload, err := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{"delta": map[string]any{"content": content}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	stream := "data: " + string(payload) + "\n" + "data: [DONE]\n"

	c := &OpenAIClient{}
	resp, err := c.parseStream("test-model", strings.NewReader(stream), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1; content=%q", len(resp.ToolCalls), resp.Content)
	}
	if resp.ToolCalls[0].Function.Name != "TodoWrite" {
		t.Fatalf("tool name = %q, want TodoWrite", resp.ToolCalls[0].Function.Name)
	}
	for _, want := range []string{`"markdown_list"`, `- [ ] 1. Inventory check`, `- [ ] 2. Cleanup namespace`} {
		if !strings.Contains(resp.ToolCalls[0].Function.Arguments, want) {
			t.Fatalf("arguments = %q, missing %q", resp.ToolCalls[0].Function.Arguments, want)
		}
	}
}

func TestParseStreamExtractsNamespacedGemmaArrayArgs(t *testing.T) {
	_ = chdirTemp(t)

	content := `call:infractl:k8s_query{action:'get',resources:['pods','services'],target:'a100'}`
	payload, err := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{"delta": map[string]any{"content": content}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	stream := "data: " + string(payload) + "\n" + "data: [DONE]\n"

	c := &OpenAIClient{}
	resp, err := c.parseStream("test-model", strings.NewReader(stream), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1; content=%q", len(resp.ToolCalls), resp.Content)
	}
	if resp.ToolCalls[0].Function.Name != "k8s_query" {
		t.Fatalf("tool name = %q, want k8s_query", resp.ToolCalls[0].Function.Name)
	}
	for _, want := range []string{`"resources":["pods","services"]`, `"target":"a100"`} {
		if !strings.Contains(resp.ToolCalls[0].Function.Arguments, want) {
			t.Fatalf("arguments = %q, missing %q", resp.ToolCalls[0].Function.Arguments, want)
		}
	}
}

func TestDoRequestWith_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"choices":[{"message":{"content":"hello"}}]}`)
	}))
	defer srv.Close()

	c := NewOpenAIClient(srv.URL, "test", "", 0)
	resp, err := c.doRequestWith(t.Context(), []byte(`{}`), c.httpClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestDoRequestWith_429ReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(429)
		fmt.Fprint(w, `{"error":{"type":"rate_limit","message":"too many requests"}}`)
	}))
	defer srv.Close()

	c := NewOpenAIClient(srv.URL, "test", "", 0)
	_, err := c.doRequestWith(t.Context(), []byte(`{}`), c.httpClient)
	if err == nil {
		t.Fatal("expected error for 429 response")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 429 {
		t.Errorf("StatusCode = %d, want 429", apiErr.StatusCode)
	}
	if apiErr.RetryAfter != 30_000_000_000 { // 30 seconds in nanoseconds
		t.Errorf("RetryAfter = %v, want 30s", apiErr.RetryAfter)
	}
	if !IsTransient(err) {
		t.Error("429 should be transient")
	}
	if !IsRateLimited(err) {
		t.Error("429 should be rate limited")
	}
}

func TestDoRequestWith_503ReturnsTransientError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		fmt.Fprint(w, `service unavailable`)
	}))
	defer srv.Close()

	c := NewOpenAIClient(srv.URL, "test", "", 0)
	_, err := c.doRequestWith(t.Context(), []byte(`{}`), c.httpClient)
	if err == nil {
		t.Fatal("expected error for 503 response")
	}

	if !IsTransient(err) {
		t.Error("503 should be transient")
	}
	if !IsOverloaded(err) {
		t.Error("503 should be overloaded")
	}
}

func TestDoRequestWith_400ReturnsNonTransientError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"error":"bad request"}`)
	}))
	defer srv.Close()

	c := NewOpenAIClient(srv.URL, "test", "", 0)
	_, err := c.doRequestWith(t.Context(), []byte(`{}`), c.httpClient)
	if err == nil {
		t.Fatal("expected error for 400 response")
	}

	if IsTransient(err) {
		t.Error("400 should NOT be transient")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want 400", apiErr.StatusCode)
	}
}

func TestDoRequestWith_500ReturnsTransientError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		fmt.Fprint(w, `internal server error`)
	}))
	defer srv.Close()

	c := NewOpenAIClient(srv.URL, "test", "", 0)
	_, err := c.doRequestWith(t.Context(), []byte(`{}`), c.httpClient)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}

	if !IsTransient(err) {
		t.Error("500 should be transient")
	}
}

func TestAdaptiveIdleTimeout(t *testing.T) {
	tests := []struct {
		name    string
		bytes   int
		wantMin time.Duration
		wantMax time.Duration
	}{
		{"small request", 1024, 90 * time.Second, 91 * time.Second},
		{"medium request 50KB", 50 * 1024, 90*time.Second + 25*time.Second, 116 * time.Second},
		{"large request 200KB", 200 * 1024, maxStreamIdleTimeout, maxStreamIdleTimeout},
		{"zero bytes", 0, defaultStreamIdleTimeout, defaultStreamIdleTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adaptiveIdleTimeout(tt.bytes)
			if got < tt.wantMin {
				t.Errorf("adaptiveIdleTimeout(%d) = %v, want >= %v", tt.bytes, got, tt.wantMin)
			}
			if got > tt.wantMax {
				t.Errorf("adaptiveIdleTimeout(%d) = %v, want <= %v", tt.bytes, got, tt.wantMax)
			}
		})
	}
}

func TestAdaptiveIdleTimeout_NeverExceedsMax(t *testing.T) {
	// 매우 큰 요청도 maxStreamIdleTimeout을 넘지 않아야 한다
	got := adaptiveIdleTimeout(10 * 1024 * 1024) // 10MB
	if got != maxStreamIdleTimeout {
		t.Errorf("expected max cap %v, got %v", maxStreamIdleTimeout, got)
	}
}

func TestDoRequestWith_SetsAuthorizationHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"choices":[]}`)
	}))
	defer srv.Close()

	c := NewOpenAIClient(srv.URL, "test", "my-api-key", 0)
	resp, err := c.doRequestWith(t.Context(), []byte(`{}`), c.httpClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if gotAuth != "Bearer my-api-key" {
		t.Errorf("Authorization = %q, want 'Bearer my-api-key'", gotAuth)
	}
}

func chdirTemp(t *testing.T) string {
	t.Helper()

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
	return tmp
}

func readLLMLog(t *testing.T, dir string) string {
	t.Helper()

	logBytes, err := os.ReadFile(filepath.Join(dir, llmLogPath))
	if err != nil {
		t.Fatal(err)
	}
	bom := []byte{0xEF, 0xBB, 0xBF}
	if !bytes.HasPrefix(logBytes, bom) {
		t.Fatalf("llm.log missing UTF-8 BOM: % x", logBytes[:min(len(logBytes), 3)])
	}
	return string(bytes.TrimPrefix(logBytes, bom))
}
