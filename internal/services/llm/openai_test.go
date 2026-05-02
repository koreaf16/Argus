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
