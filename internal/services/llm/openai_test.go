package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type capturedEmitter struct {
	text     strings.Builder
	thinking strings.Builder
}

func TestOpenAICompatStreamPrependsSystemMessage(t *testing.T) {
	var payload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer srv.Close()

	client := NewOpenAICompat(ModelEntry{
		ModelID: "test-model",
		BaseURL: srv.URL,
	}, "", srv.Client())
	stream, err := client.Stream(context.Background(), Request{
		System: []SystemBlock{
			{Type: "text", Text: "core rules"},
			{Type: "text", Text: "dynamic context"},
		},
		Messages: []Message{TextMessage(RoleUser, "hello")},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	for evt := range stream {
		if evt.Kind == EventError {
			t.Fatalf("stream error: %v", evt.Err)
		}
	}

	messages, ok := payload["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %#v", payload["messages"])
	}
	systemMsg, ok := messages[0].(map[string]any)
	if !ok {
		t.Fatalf("first message is not an object: %#v", messages[0])
	}
	if systemMsg["role"] != "system" {
		t.Fatalf("first message role = %v, want system", systemMsg["role"])
	}
	content, _ := systemMsg["content"].(string)
	if !strings.Contains(content, "core rules") || !strings.Contains(content, "dynamic context") {
		t.Fatalf("system content missing blocks: %q", content)
	}
}

func (c *capturedEmitter) EmitText(s string)     { c.text.WriteString(s) }
func (c *capturedEmitter) EmitThinking(s string) { c.thinking.WriteString(s) }

func TestChannelTokenFilter_SeparatesTextAndThinking(t *testing.T) {
	tests := []struct {
		name         string
		chunks       []string
		wantText     string
		wantThinking string
	}{
		{
			name:         "double-pipe final channel emits text",
			chunks:       []string{"<|channel|>final<|message|>안녕<|end|>"},
			wantText:     "안녕",
			wantThinking: "",
		},
		{
			name:         "right-pipe token with adjacent content",
			chunks:       []string{"<channel|>현재 사용 중인 디스크"},
			wantText:     "현재 사용 중인 디스크",
			wantThinking: "",
		},
		{
			name:         "left-pipe channel label line stripped",
			chunks:       []string{"<|channel>thought\n본문"},
			wantText:     "본문",
			wantThinking: "",
		},
		{
			name:         "standalone channel label line removed",
			chunks:       []string{"hello\nanalysis\nworld\n"},
			wantText:     "hello\nworld\n",
			wantThinking: "",
		},
		{
			name:         "standalone thought label removed",
			chunks:       []string{"thought\n"},
			wantText:     "",
			wantThinking: "",
		},
		{
			name:         "plain text unchanged",
			chunks:       []string{"Hello world\n"},
			wantText:     "Hello world\n",
			wantThinking: "",
		},
		{
			name:         "control token across chunks",
			chunks:       []string{"before <|chan", "nel|>final<|message|> after<|end|>"},
			wantText:     "before  after",
			wantThinking: "",
		},
		{
			name:         "harmony analysis routes to thinking, final to text",
			chunks:       []string{"<|channel|>analysis<|message|>thinking<|end|><|channel|>final<|message|>답변<|return|>"},
			wantText:     "답변",
			wantThinking: "thinking",
		},
		{
			name:         "think tag routes body to thinking",
			chunks:       []string{"<think>본문</think>답변"},
			wantText:     "답변",
			wantThinking: "본문",
		},
		{
			name:         "think tag split across chunks",
			chunks:       []string{"<think>분", "석 중</think>결과"},
			wantText:     "결과",
			wantThinking: "분석 중",
		},
		{
			name:         "harmony body across chunks",
			chunks:       []string{"<|channel|>analysis<|message|>A", "B<|end|><|channel|>final<|message|>C<|end|>"},
			wantText:     "C",
			wantThinking: "AB",
		},
		{
			name:         "channel token split across chunks",
			chunks:       []string{"<|chan", "nel|>analysis<|message|>x<|end|>"},
			wantText:     "",
			wantThinking: "x",
		},
		{
			name:         "regular less-than not held",
			chunks:       []string{"x < 5 is true"},
			wantText:     "x < 5 is true",
			wantThinking: "",
		},
		{
			name:         "control token at line start with content after",
			chunks:       []string{"<|message|>Hello\n<|end|>"},
			wantText:     "Hello\n",
			wantThinking: "",
		},
		{
			name:         "think close split across chunks",
			chunks:       []string{"<think>중간</thi", "nk>최종"},
			wantText:     "최종",
			wantThinking: "중간",
		},
		{
			name:         "commentary channel routes to thinking",
			chunks:       []string{"<|channel|>commentary<|message|>주석<|end|>본문"},
			wantText:     "본문",
			wantThinking: "주석",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &channelTokenFilter{}
			cap := &capturedEmitter{}
			for _, chunk := range tc.chunks {
				f.feed(chunk, cap)
			}
			f.flush(cap)
			if got := cap.text.String(); got != tc.wantText {
				t.Errorf("text: got %q, want %q", got, tc.wantText)
			}
			if got := cap.thinking.String(); got != tc.wantThinking {
				t.Errorf("thinking: got %q, want %q", got, tc.wantThinking)
			}
		})
	}
}

func TestIsGemma4Model(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"/models/gemma-4-26B-A4B-it-AWQ-4bit", true},
		{"google/gemma-4-31b-it", true},
		{"GEMMA-4-27B", true},
		{"gemma4-26b", true},
		{"gemma4", true},
		{"gemma-3-27b-it", false},
		{"gemma-3", false},
		{"gemma3", false},
		{"qwen3.6-35b", false},
		{"claude-opus-4-7", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := isGemma4Model(tc.model); got != tc.want {
			t.Errorf("isGemma4Model(%q) = %v, want %v", tc.model, got, tc.want)
		}
	}
}

func captureOpenAIPayload(t *testing.T, model string, req Request) map[string]any {
	t.Helper()
	var payload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n")
	}))
	t.Cleanup(srv.Close)

	client := NewOpenAICompat(ModelEntry{
		ModelID: model,
		BaseURL: srv.URL,
	}, "", srv.Client())
	stream, err := client.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	for evt := range stream {
		if evt.Kind == EventError {
			t.Fatalf("stream error: %v", evt.Err)
		}
	}
	return payload
}

func TestOpenAICompatGemma4InjectsSamplingDefaults(t *testing.T) {
	payload := captureOpenAIPayload(t, "/models/gemma-4-26B-A4B-it-AWQ-4bit", Request{
		Messages: []Message{TextMessage(RoleUser, "hi")},
	})

	wants := map[string]float64{
		"repetition_penalty": 1.3,
		"frequency_penalty":  0.5,
		"presence_penalty":   0.3,
		"top_k":              50,
		"min_p":              0.05,
		"temperature":        0.8,
		"top_p":              0.9,
	}
	for key, want := range wants {
		got, ok := payload[key].(float64)
		if !ok {
			t.Errorf("payload[%q] missing or not number: %#v", key, payload[key])
			continue
		}
		if got != want {
			t.Errorf("payload[%q] = %v, want %v", key, got, want)
		}
	}
}

func TestOpenAICompatGemma4UserOverridesWin(t *testing.T) {
	temp := 0.2
	topP := 0.5
	payload := captureOpenAIPayload(t, "gemma4-26b", Request{
		Messages:    []Message{TextMessage(RoleUser, "hi")},
		Temperature: &temp,
		TopP:        &topP,
	})
	if got, _ := payload["temperature"].(float64); got != 0.2 {
		t.Errorf("temperature = %v, want 0.2 (user value)", payload["temperature"])
	}
	if got, _ := payload["top_p"].(float64); got != 0.5 {
		t.Errorf("top_p = %v, want 0.5 (user value)", payload["top_p"])
	}
	// 사용자가 제공하지 않은 보정 파라미터는 여전히 주입되어야 함
	if got, _ := payload["repetition_penalty"].(float64); got != 1.3 {
		t.Errorf("repetition_penalty = %v, want 1.3", payload["repetition_penalty"])
	}
}

func TestOpenAICompatNonGemmaSkipsSamplingInjection(t *testing.T) {
	payload := captureOpenAIPayload(t, "claude-opus-4-7", Request{
		Messages: []Message{TextMessage(RoleUser, "hi")},
	})
	for _, key := range []string{"repetition_penalty", "top_k", "min_p"} {
		if _, present := payload[key]; present {
			t.Errorf("payload[%q] should be absent for non-Gemma model: %#v", key, payload[key])
		}
	}
	// 사용자 미설정 + 비-Gemma → temperature/top_p도 주입되지 않아야 함
	if _, present := payload["temperature"]; present {
		t.Errorf("temperature should be absent for non-Gemma model without user override")
	}
}

func TestIsQwen36Model(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"/models/Qwen3.6-35B-A3B-Claude-4.7-Opus-Reasoning-Distilled-AWQ-INT4", true},
		{"qwen3.6-35b", true},
		{"qwen3.5-35b-a3b", true},
		{"Qwen3-6-35B", true},
		{"qwen3-5-9b", true},
		{"qwen3-30b-a3b", false},
		{"qwen2.5-72b", false},
		{"qwen", false},
		{"gemma-4-26b", false},
		{"claude-opus-4-7", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := isQwen36Model(tc.model); got != tc.want {
			t.Errorf("isQwen36Model(%q) = %v, want %v", tc.model, got, tc.want)
		}
	}
}

func TestOpenAICompatQwen36InjectsAntiLoopDefaults(t *testing.T) {
	payload := captureOpenAIPayload(t, "/models/Qwen3.6-35B-A3B-Claude-4.7-Opus-Reasoning-Distilled-AWQ-INT4", Request{
		Messages: []Message{TextMessage(RoleUser, "hi")},
	})

	wants := map[string]float64{
		"temperature":        0.2,
		"top_p":              0.95,
		"top_k":              20,
		"min_p":              0.0,
		"presence_penalty":   1.5,
		"repetition_penalty": 1.0,
	}
	for key, want := range wants {
		got, ok := payload[key].(float64)
		if !ok {
			t.Errorf("payload[%q] missing for Qwen3.6: %#v", key, payload[key])
			continue
		}
		if got != want {
			t.Errorf("payload[%q] = %v, want %v", key, got, want)
		}
	}
	// Qwen3.6은 frequency_penalty default를 두지 않음 (Qwen 공식 권장에 없음)
	if _, present := payload["frequency_penalty"]; present {
		t.Errorf("frequency_penalty should be absent for Qwen3.6: got %#v", payload["frequency_penalty"])
	}
}

func TestOpenAICompatQwenLegacyKeepsZeroPenalty(t *testing.T) {
	payload := captureOpenAIPayload(t, "qwen2.5-72b", Request{
		Messages: []Message{TextMessage(RoleUser, "hi")},
	})
	for _, key := range []string{"presence_penalty", "frequency_penalty"} {
		got, ok := payload[key].(float64)
		if !ok {
			t.Errorf("payload[%q] missing for legacy Qwen: %#v", key, payload[key])
			continue
		}
		if got != 0.0 {
			t.Errorf("payload[%q] = %v, want 0.0 (legacy Qwen)", key, got)
		}
	}
	if _, present := payload["repetition_penalty"]; present {
		t.Errorf("repetition_penalty should not be set for legacy Qwen")
	}
	if _, present := payload["temperature"]; present {
		t.Errorf("legacy Qwen should not get temperature default: got %#v", payload["temperature"])
	}
}

func TestOpenAICompatModelEntrySamplingOverride(t *testing.T) {
	temp := 0.5
	minP := 0.2
	var payload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n")
	}))
	t.Cleanup(srv.Close)

	client := NewOpenAICompat(ModelEntry{
		ModelID: "/models/Qwen3.6-35B-A3B-test",
		BaseURL: srv.URL,
		Sampling: &SamplingParams{
			Temperature: &temp,
			MinP:        &minP,
		},
	}, "", srv.Client())
	stream, err := client.Stream(context.Background(), Request{
		Messages: []Message{TextMessage(RoleUser, "hi")},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for evt := range stream {
		if evt.Kind == EventError {
			t.Fatalf("stream error: %v", evt.Err)
		}
	}

	// ModelEntry override가 model default를 덮어씀
	if got, _ := payload["temperature"].(float64); got != 0.5 {
		t.Errorf("temperature = %v, want 0.5 (entry override)", payload["temperature"])
	}
	if got, _ := payload["min_p"].(float64); got != 0.2 {
		t.Errorf("min_p = %v, want 0.2 (entry override)", payload["min_p"])
	}
	// 미지정 필드는 model default 유지
	if got, _ := payload["presence_penalty"].(float64); got != 1.5 {
		t.Errorf("presence_penalty = %v, want 1.5 (Qwen3.6 default)", payload["presence_penalty"])
	}
}

func TestOpenAICompatRequestOverridesEntrySampling(t *testing.T) {
	entryTemp := 0.5
	reqTemp := 0.9
	var payload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n")
	}))
	t.Cleanup(srv.Close)

	client := NewOpenAICompat(ModelEntry{
		ModelID:  "/models/Qwen3.6-35B-test",
		BaseURL:  srv.URL,
		Sampling: &SamplingParams{Temperature: &entryTemp},
	}, "", srv.Client())
	stream, err := client.Stream(context.Background(), Request{
		Messages:    []Message{TextMessage(RoleUser, "hi")},
		Temperature: &reqTemp,
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for evt := range stream {
		if evt.Kind == EventError {
			t.Fatalf("stream error: %v", evt.Err)
		}
	}

	if got, _ := payload["temperature"].(float64); got != 0.9 {
		t.Errorf("temperature = %v, want 0.9 (request beats entry)", payload["temperature"])
	}
}

func TestSplitAtPotentialToken(t *testing.T) {
	tests := []struct {
		text     string
		wantSafe string
		wantHeld string
	}{
		{"hello <|chann", "hello ", "<|chann"},
		{"hello <channel", "hello ", "<channel"},
		{"hello < world", "hello < world", ""},
		{"<|channel|> done", "<|channel|> done", ""},
		{"no angle bracket", "no angle bracket", ""},
		{"text <|done|> more", "text <|done|> more", ""},
		{"prefix </thi", "prefix ", "</thi"},
		{"prefix <thi", "prefix ", "<thi"},
	}
	for _, tc := range tests {
		safe, held := splitAtPotentialToken(tc.text)
		if safe != tc.wantSafe || held != tc.wantHeld {
			t.Errorf("splitAtPotentialToken(%q) = (%q, %q), want (%q, %q)",
				tc.text, safe, held, tc.wantSafe, tc.wantHeld)
		}
	}
}

func TestSplitAtPotentialThinkClose(t *testing.T) {
	tests := []struct {
		text     string
		wantSafe string
		wantHeld string
	}{
		{"body </thi", "body ", "</thi"},
		{"body </think", "body ", "</think"},
		{"body </think> tail", "body </think> tail", ""},
		{"plain body", "plain body", ""},
		{"body <not", "body <not", ""},
	}
	for _, tc := range tests {
		safe, held := splitAtPotentialThinkClose(tc.text)
		if safe != tc.wantSafe || held != tc.wantHeld {
			t.Errorf("splitAtPotentialThinkClose(%q) = (%q, %q), want (%q, %q)",
				tc.text, safe, held, tc.wantSafe, tc.wantHeld)
		}
	}
}
