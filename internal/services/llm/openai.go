package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

type openAICompatLLM struct {
	entry      ModelEntry
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewOpenAICompat(entry ModelEntry, apiKey string, httpClient *http.Client) LLM {
	baseURL := strings.TrimRight(entry.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &openAICompatLLM{
		entry:      entry,
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

func (o *openAICompatLLM) Provider() string { return string(ProviderOpenAICompat) }

func (o *openAICompatLLM) Capabilities() Caps { return o.entry.Caps }

func (o *openAICompatLLM) CountTokens(ctx context.Context, req Request) (int, error) {
	_ = ctx
	return ApproximateTokenCount(req), nil
}

func (o *openAICompatLLM) Stream(ctx context.Context, req Request) (<-chan Event, error) {
	model := defaultIfEmpty(req.Model, o.entry.ModelID)
	payload := map[string]any{
		"model":    model,
		"messages": toOpenAIMessages(req.Messages, req.Thinking != nil),
		"stream":   true,
	}
	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
	}
	if len(req.Tools) > 0 {
		payload["tools"] = toOpenAITools(req.Tools)
	}

	// Thinking 설정이 있는 경우 페이로드에 추가
	if req.Thinking != nil {
		payload["thinking"] = req.Thinking
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if req.TraceHook != nil {
		reqCopy := req
		reqCopy.TraceHook = nil
		req.TraceHook(TraceEvent{
			Kind:     TraceKindRequest,
			Provider: o.Provider(),
			Model:    model,
			Raw:      copyRaw(body),
			Request:  &reqCopy,
		})
	}
	endpoint := strings.TrimRight(o.baseURL, "/") + "/chat/completions"
	apiKey := o.apiKey
	makeReq := func() (*http.Request, error) {
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		r.Header.Set("content-type", "application/json")
		r.Header.Set("accept", "text/event-stream")
		if strings.TrimSpace(apiKey) != "" {
			r.Header.Set("authorization", "Bearer "+apiKey)
		}
		return r, nil
	}

	streamClient := &http.Client{Transport: o.httpClient.Transport, Timeout: 0}
	resp, err := doWithRetry(ctx, streamClient, makeReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("openai-compatible stream failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	out := make(chan Event, 64)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		accumulator := NewStreamAccumulator()
		filter := &channelTokenFilter{}

		emitText := func(raw, delta string) {
			filtered := filter.feed(delta)
			if filtered == "" {
				return
			}
			if req.TraceHook != nil {
				req.TraceHook(TraceEvent{
					Kind:        TraceKindProviderChunk,
					Provider:    o.Provider(),
					Model:       model,
					Raw:         copyRaw([]byte(raw)),
					MappedKind:  EventTextDelta,
					MappedDelta: filtered,
				})
			}
			out <- Event{Kind: EventTextDelta, Delta: filtered}
		}

		flushFilter := func(raw string) {
			if remaining := filter.flush(); remaining != "" {
				if req.TraceHook != nil {
					req.TraceHook(TraceEvent{
						Kind:        TraceKindProviderChunk,
						Provider:    o.Provider(),
						Model:       model,
						Raw:         copyRaw([]byte(raw)),
						MappedKind:  EventTextDelta,
						MappedDelta: remaining,
					})
				}
				out <- Event{Kind: EventTextDelta, Delta: remaining}
			}
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "" {
				continue
			}
			if payload == "[DONE]" {
				flushFilter(payload)
				stop := StopReasonEndTurn
				out <- Event{Kind: EventStop, Stop: &stop}
				return
			}

			var chunk openAIChunk
			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				if req.TraceHook != nil {
					req.TraceHook(TraceEvent{
						Kind:       TraceKindProviderChunk,
						Provider:   o.Provider(),
						Model:      model,
						Raw:        copyRaw([]byte(payload)),
						MappedKind: EventError,
					})
				}
				out <- Event{Kind: EventError, Err: err}
				continue
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			choice := chunk.Choices[0]
			if choice.Delta.Content != "" {
				emitText(payload, choice.Delta.Content)
			}
			for _, tc := range choice.Delta.ToolCalls {
				accumulator.AppendOpenAIToolDelta(tc.Index, tc.ID, tc.Function.Name, tc.Function.Arguments)
			}

			if choice.FinishReason == "tool_calls" {
				flushFilter(payload)
				for _, tc := range accumulator.FlushOpenAIToolUses() {
					if req.TraceHook != nil {
						req.TraceHook(TraceEvent{
							Kind:           TraceKindProviderChunk,
							Provider:       o.Provider(),
							Model:          model,
							Raw:            copyRaw([]byte(payload)),
							MappedKind:     EventToolUseStart,
							MappedToolName: tc.Name,
						})
					}
					out <- Event{
						Kind: EventToolUseStart,
						ToolUse: &ToolUseStart{
							ID:    tc.ID,
							Name:  tc.Name,
							Input: tc.Input,
						},
					}
				}
				stop := StopReasonToolUse
				if req.TraceHook != nil {
					req.TraceHook(TraceEvent{
						Kind:       TraceKindProviderChunk,
						Provider:   o.Provider(),
						Model:      model,
						Raw:        copyRaw([]byte(payload)),
						MappedKind: EventStop,
						MappedStop: &stop,
					})
				}
				out <- Event{Kind: EventStop, Stop: &stop}
				continue
			}

			if choice.FinishReason != "" {
				flushFilter(payload)
				stop := mapOpenAIStop(choice.FinishReason)
				if req.TraceHook != nil {
					req.TraceHook(TraceEvent{
						Kind:       TraceKindProviderChunk,
						Provider:   o.Provider(),
						Model:      model,
						Raw:        copyRaw([]byte(payload)),
						MappedKind: EventStop,
						MappedStop: &stop,
					})
				}
				out <- Event{Kind: EventStop, Stop: &stop}
				return
			}
		}
		if err := scanner.Err(); err != nil {
			out <- Event{Kind: EventError, Err: err}
			return
		}
		stop := StopReasonEndTurn
		out <- Event{Kind: EventStop, Stop: &stop}
	}()

	return out, nil
}

func copyRaw(in []byte) json.RawMessage {
	if len(in) == 0 {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}

type openAIChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func mapOpenAIStop(s string) StopReason {
	switch s {
	case "stop":
		return StopReasonEndTurn
	case "tool_calls":
		return StopReasonToolUse
	case "length":
		return StopReasonMaxTokens
	default:
		return StopReasonUnknown
	}
}

func toOpenAIMessages(msgs []Message, thinkingEnabled bool) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	for i, m := range msgs {
		if m.Role == RoleUser {
			// ... (기존 유저 메시지 처리 로직 동일)
			texts := make([]string, 0, len(m.Content))
			for _, b := range m.Content {
				if b.Type == ContentToolResult {
					out = append(out, map[string]any{
						"role":         "tool",
						"tool_call_id": b.ToolUseID,
						"content":      b.Text,
					})
					continue
				}
				if b.Type == ContentText && b.Text != "" {
					texts = append(texts, b.Text)
				}
			}
			if len(texts) > 0 {
				out = append(out, map[string]any{
					"role":    "user",
					"content": strings.Join(texts, "\n"),
				})
			}
			continue
		}

		// Assistant 메시지 처리
		// Thinking 모드가 활성화된 경우 마지막 Assistant 메시지(Prefill)를 무시함
		if thinkingEnabled && i == len(msgs)-1 && m.Role == RoleAssistant {
			continue
		}

		texts := make([]string, 0, len(m.Content))
		toolCalls := make([]map[string]any, 0, len(m.Content))
		for _, b := range m.Content {
			switch b.Type {
			case ContentText:
				if b.Text != "" {
					texts = append(texts, b.Text)
				}
			case ContentToolUse:
				args := "{}"
				if len(b.Input) > 0 && json.Valid(b.Input) {
					args = string(b.Input)
				}
				toolCalls = append(toolCalls, map[string]any{
					"id":   b.ID,
					"type": "function",
					"function": map[string]any{
						"name":      b.Name,
						"arguments": args,
					},
				})
			}
		}

		// 빈 Assistant 메시지 필터링
		if i == len(msgs)-1 && m.Role == RoleAssistant && len(texts) == 0 && len(toolCalls) == 0 {
			continue
		}

		msg := map[string]any{
			"role":    "assistant",
			"content": strings.Join(texts, "\n"),
		}
		if len(toolCalls) > 0 {
			msg["tool_calls"] = toolCalls
		}
		out = append(out, msg)
	}
	return out
}

// channelTokenFilter는 OpenAI Harmony / Gemma 등 일부 모델이 응답 본문에 흘려보내는
// 채널/메시지 특수 토큰을 제거합니다. 인라인 토큰과 표준 단독-라벨 라인 모두 처리하며,
// chunk 경계에서 토큰이 잘리는 경우 lookahead 버퍼로 다음 chunk와 합쳐 처리합니다.
type channelTokenFilter struct {
	pendingToken string // chunk 경계에서 잘린 미완성 토큰 (예: "<|chan")
	pendingLine  string // 단독-라벨 판별을 위한 줄 단위 버퍼 (개행 전)
}

// harmonyInlineRe: 한 번에 제거할 수 있는 인라인 토큰들.
//
// 1) <|channel|>(analysis|commentary|final|thought) — 채널 토큰 + 라벨까지 한꺼번에 소거
// 2) <|channel>...\n — 좌측 단일 파이프 변형: 해당 라인 전체를 소거
// 3) <channel|> — 우측 단일 파이프 변형
// 4) <|(channel|message|end|return|start|stop|done|assistant|user|system|developer|tool)|> — 기타 토큰
var harmonyInlineRe = regexp.MustCompile(
	`<\|channel\|>(?:analysis|commentary|final|thought)` +
		`|<\|channel>[^\n]*\n?` +
		`|<channel\|>` +
		`|<\|(?:channel|message|end|return|start|stop|done|assistant|user|system|developer|tool)\|>`,
)

// 단독으로 한 줄에 등장하면 제거할 Harmony 채널 라벨.
var harmonyStandaloneLabel = map[string]bool{
	"analysis":   true,
	"thought":    true,
	"commentary": true,
	"final":      true,
}

// chunk 경계에서 부분 토큰일 가능성이 있는지 판단할 때 사용하는 알려진 키워드 목록.
var harmonyTokenKeywords = []string{
	"channel", "message", "end", "return", "start", "stop", "done",
	"assistant", "user", "system", "developer", "tool",
}

func (f *channelTokenFilter) feed(delta string) string {
	text := f.pendingToken + delta
	f.pendingToken = ""

	safe, held := splitAtPotentialToken(text)
	f.pendingToken = held

	// 인라인 토큰 제거.
	safe = harmonyInlineRe.ReplaceAllString(safe, "")

	// 단독 라벨 판별을 위해 줄 단위로 잘라 처리.
	combined := f.pendingLine + safe
	f.pendingLine = ""

	lastNL := strings.LastIndex(combined, "\n")
	if lastNL == -1 {
		f.pendingLine = combined
		return ""
	}
	complete := combined[:lastNL+1]
	f.pendingLine = combined[lastNL+1:]

	return removeStandaloneLabelLines(complete)
}

func (f *channelTokenFilter) flush() string {
	// 미완성 토큰은 폐기 (어차피 사용자에게 노출되면 안 되는 garbage).
	f.pendingToken = ""

	line := f.pendingLine
	f.pendingLine = ""
	if line == "" {
		return ""
	}
	line = harmonyInlineRe.ReplaceAllString(line, "")
	if harmonyStandaloneLabel[strings.TrimSpace(line)] {
		return ""
	}
	return line
}

// applyHarmonyFilter는 한 번에 들어온 전체 문자열에 대해 동일한 필터링을 적용합니다.
func applyHarmonyFilter(s string) string {
	if s == "" {
		return ""
	}
	s = harmonyInlineRe.ReplaceAllString(s, "")
	return removeStandaloneLabelLines(s)
}

// splitAtPotentialToken은 chunk 끝 부분이 미완성 Harmony 토큰처럼 보일 경우
// 해당 부분을 held로 분리해 다음 chunk와 합쳐 재처리하도록 합니다.
func splitAtPotentialToken(text string) (safe, held string) {
	if text == "" {
		return "", ""
	}
	lastLT := strings.LastIndex(text, "<")
	if lastLT == -1 {
		return text, ""
	}
	suffix := text[lastLT:]
	if strings.Contains(suffix, ">") {
		return text, ""
	}
	if isPartialHarmonyToken(suffix) {
		return text[:lastLT], suffix
	}
	return text, ""
}

func isPartialHarmonyToken(s string) bool {
	if len(s) == 0 || s[0] != '<' {
		return false
	}
	if len(s) == 1 {
		return true
	}
	rest := s[1:]
	if rest[0] == '|' {
		rest = rest[1:]
	}
	if rest == "" {
		return true
	}
	rest = strings.TrimRight(rest, "|")
	if rest == "" {
		return true
	}
	for _, r := range rest {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	for _, kw := range harmonyTokenKeywords {
		if strings.HasPrefix(kw, rest) {
			return true
		}
	}
	return false
}

func removeStandaloneLabelLines(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '\n' {
			line := s[start:i]
			if !harmonyStandaloneLabel[strings.TrimSpace(line)] {
				b.WriteString(line)
				if i < len(s) {
					b.WriteByte('\n')
				}
			}
			start = i + 1
		}
	}
	return b.String()
}

func toOpenAITools(specs []ToolSpec) []map[string]any {
	out := make([]map[string]any, 0, len(specs))
	for _, s := range specs {
		if s.Name == "" {
			continue
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        s.Name,
				"description": s.Description,
				"parameters":  s.InputSchema,
			},
		})
	}
	return out
}
