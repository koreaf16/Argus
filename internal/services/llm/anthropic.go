// Package llm — Anthropic Messages API streaming adapter.
//
// 파일 역할: Anthropic Messages SSE를 내부 LLM 이벤트 형식으로 변환한다.
// tool_use/text/thinking/stop 이벤트를 파싱하고 Query Engine이 소비 가능한 Event를 생성한다.
// 포함 모듈:
//   - anthropicLLM: Anthropic provider 구현체.
//   - decodeAnthropicEvent: SSE payload 디코더.
//   - toAnthropicMessages/toAnthropicTools: 내부 요청 모델 -> Anthropic 요청 변환.
//
// 호출/사용 방식:
//   - internal/services/llm/registry.go 의 Build()에서 NewAnthropic()로 생성한다.
//   - 외부 진입점은 NewAnthropic(), (*anthropicLLM).Stream().
//
// 연결:
//   - import 하는 주요 패키지 (internal/...): 없음 (동일 패키지 타입 사용).
//   - 이 파일을 import 하는 주요 패키지: internal/services/llm/registry.go.
// 파일 역할: Anthropic 모델과의 통신을 처리하는 LLM 서비스 구현체.
// 포함 모듈: llm, net/http, encoding/json 등.
// 호출/사용 방식: llm.Registry를 통해 서비스 인터페이스를 구현하여 사용.
// 연결: internal/services/llm/llm.go 기반.

package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type anthropicLLM struct {
	entry      ModelEntry
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewAnthropic(entry ModelEntry, apiKey string, httpClient *http.Client) LLM {
	baseURL := strings.TrimRight(entry.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &anthropicLLM{
		entry:      entry,
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

func (a *anthropicLLM) Provider() string { return string(ProviderAnthropic) }

func (a *anthropicLLM) Capabilities() Caps { return a.entry.Caps }

func (a *anthropicLLM) CountTokens(ctx context.Context, req Request) (int, error) {
	_ = ctx
	return ApproximateTokenCount(req), nil
}

func (a *anthropicLLM) Stream(ctx context.Context, req Request) (<-chan Event, error) {
	if strings.TrimSpace(a.apiKey) == "" {
		return nil, fmt.Errorf("missing anthropic api key")
	}
	model := defaultIfEmpty(req.Model, a.entry.ModelID)
	endpoint := strings.TrimRight(a.baseURL, "/") + "/messages"
	payload := map[string]any{
		"model":      model,
		"max_tokens": maxOrDefault(req.MaxTokens, 2048),
		"messages":   toAnthropicMessages(req.Messages),
		"stream":     true,
	}
	if len(req.System) > 0 {
		payload["system"] = toAnthropicSystem(req.System)
	}
	if len(req.Tools) > 0 {
		payload["tools"] = toAnthropicTools(req.Tools)
	}
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
			Provider: a.Provider(),
			Model:    model,
			Raw:      copyRaw(body),
			Request:  &reqCopy,
		})
	}
	makeReq := func() (*http.Request, error) {
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		r.Header.Set("content-type", "application/json")
		r.Header.Set("x-api-key", a.apiKey)
		r.Header.Set("anthropic-version", "2023-06-01")
		r.Header.Set("accept", "text/event-stream")
		return r, nil
	}

	// 타임아웃 없는 클라이언트로 스트림 수신 (타임아웃은 호출 ctx로 제어)
	streamClient := &http.Client{Transport: a.httpClient.Transport, Timeout: 0}
	resp, err := doWithRetry(ctx, streamClient, makeReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("anthropic stream failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	out := make(chan Event, 64)
	go func() {
		defer close(out)
		defer resp.Body.Close()

		decoder := newAnthropicStreamDecoder()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "" || payload == "[DONE]" {
				continue
			}
			if events, ok := decoder.Decode(payload); ok {
				for _, evt := range events {
					if req.TraceHook != nil {
						trace := TraceEvent{
							Kind:       TraceKindProviderChunk,
							Provider:   a.Provider(),
							Model:      model,
							Raw:        copyRaw([]byte(payload)),
							MappedKind: evt.Kind,
						}
						if evt.Delta != "" {
							trace.MappedDelta = evt.Delta
						}
						if evt.ToolUse != nil {
							trace.MappedToolName = evt.ToolUse.Name
						}
						if evt.Stop != nil {
							trace.MappedStop = evt.Stop
						}
						req.TraceHook(trace)
					}
					out <- evt
				}
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

type anthropicStreamDecoder struct {
	tools map[int]*anthropicToolUseAccum
}

type anthropicToolUseAccum struct {
	id           string
	name         string
	initialInput string
	args         strings.Builder
	seenDelta    bool
}

func newAnthropicStreamDecoder() *anthropicStreamDecoder {
	return &anthropicStreamDecoder{
		tools: make(map[int]*anthropicToolUseAccum),
	}
}

func (d *anthropicStreamDecoder) Decode(raw string) ([]Event, bool) {
	evt, ok := d.decode(raw)
	if !ok {
		return nil, false
	}
	return []Event{evt}, true
}

func (d *anthropicStreamDecoder) decode(raw string) (Event, bool) {
	var envelope map[string]any
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return Event{Kind: EventError, Err: err}, true
	}
	switch envelope["type"] {
	case "content_block_delta":
		delta, _ := envelope["delta"].(map[string]any)
		switch delta["type"] {
		case "text_delta":
			text, _ := delta["text"].(string)
			if text == "" {
				return Event{}, false
			}
			return Event{Kind: EventTextDelta, Delta: text}, true
		case "thinking_delta":
			text, _ := delta["thinking"].(string)
			if text == "" {
				return Event{}, false
			}
			return Event{Kind: EventThinkingDelta, Delta: text}, true
		case "input_json_delta":
			partial, _ := delta["partial_json"].(string)
			if partial == "" {
				return Event{}, false
			}
			idx := anthropicEventIndex(envelope)
			acc := d.tools[idx]
			if acc == nil {
				acc = &anthropicToolUseAccum{}
				d.tools[idx] = acc
			}
			acc.args.WriteString(partial)
			acc.seenDelta = true
			return Event{}, false
		default:
			return Event{}, false
		}
	case "content_block_start":
		block, _ := envelope["content_block"].(map[string]any)
		if block["type"] != "tool_use" {
			return Event{}, false
		}
		idx := anthropicEventIndex(envelope)
		acc := &anthropicToolUseAccum{
			id:   asString(block["id"]),
			name: asString(block["name"]),
		}
		if input, ok := block["input"]; ok {
			if b, err := json.Marshal(input); err == nil && string(b) != "{}" {
				acc.initialInput = string(b)
			}
		}
		d.tools[idx] = acc
		return Event{}, false
	case "content_block_stop":
		idx := anthropicEventIndex(envelope)
		acc := d.tools[idx]
		if acc == nil {
			return Event{}, false
		}
		delete(d.tools, idx)
		input := strings.TrimSpace(acc.initialInput)
		if acc.seenDelta {
			input = strings.TrimSpace(acc.args.String())
		}
		return Event{
			Kind: EventToolUseStart,
			ToolUse: &ToolUseStart{
				ID:    acc.id,
				Name:  acc.name,
				Input: normalizeToolInputJSON(input),
			},
		}, true
	case "message_delta":
		delta, _ := envelope["delta"].(map[string]any)
		stop := mapAnthropicStop(asString(delta["stop_reason"]))
		if stop == "" {
			return Event{}, false
		}
		return Event{Kind: EventStop, Stop: &stop}, true
	case "message_stop":
		stop := StopReasonEndTurn
		return Event{Kind: EventStop, Stop: &stop}, true
	case "error":
		errObj, _ := envelope["error"].(map[string]any)
		msg := asString(errObj["message"])
		if msg == "" {
			msg = "anthropic stream error"
		}
		return Event{Kind: EventError, Err: fmt.Errorf("%s", msg)}, true
	default:
		return Event{}, false
	}
}

func anthropicEventIndex(envelope map[string]any) int {
	switch v := envelope["index"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

func normalizeToolInputJSON(input string) json.RawMessage {
	input = strings.TrimSpace(input)
	if input == "" {
		return json.RawMessage(`{}`)
	}
	if json.Valid([]byte(input)) {
		return json.RawMessage(input)
	}
	if wrapped, err := json.Marshal(map[string]string{"raw": input}); err == nil {
		return json.RawMessage(wrapped)
	}
	return json.RawMessage(`{}`)
}

func mapAnthropicStop(s string) StopReason {
	switch s {
	case "end_turn":
		return StopReasonEndTurn
	case "tool_use":
		return StopReasonToolUse
	case "max_tokens":
		return StopReasonMaxTokens
	default:
		return StopReasonUnknown
	}
}

func toAnthropicSystem(blocks []SystemBlock) []map[string]any {
	out := make([]map[string]any, 0, len(blocks))
	for _, b := range blocks {
		item := map[string]any{
			"type": defaultIfEmpty(b.Type, "text"),
			"text": b.Text,
		}
		if len(b.CacheControl) > 0 {
			item["cache_control"] = b.CacheControl
		}
		out = append(out, item)
	}
	return out
}

func toAnthropicMessages(msgs []Message) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, map[string]any{
			"role":    m.Role,
			"content": toAnthropicContent(m.Content),
		})
	}
	return out
}

func toAnthropicContent(blocks []ContentBlock) []map[string]any {
	out := make([]map[string]any, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case ContentText, ContentThinking:
			out = append(out, map[string]any{
				"type": "text",
				"text": b.Text,
			})
		case ContentToolUse:
			var input any = map[string]any{}
			if len(b.Input) > 0 {
				_ = json.Unmarshal(b.Input, &input)
			}
			out = append(out, map[string]any{
				"type":  "tool_use",
				"id":    b.ID,
				"name":  b.Name,
				"input": input,
			})
		case ContentToolResult:
			out = append(out, map[string]any{
				"type":        "tool_result",
				"tool_use_id": b.ToolUseID,
				"content": []map[string]any{
					{"type": "text", "text": b.Text},
				},
				"is_error": b.IsError,
			})
		}
	}
	return out
}

func toAnthropicTools(specs []ToolSpec) []map[string]any {
	out := make([]map[string]any, 0, len(specs))
	for _, t := range specs {
		out = append(out, map[string]any{
			"name":         t.Name,
			"description":  t.Description,
			"input_schema": t.InputSchema,
		})
	}
	return out
}

func defaultIfEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func maxOrDefault(v, fallback int) int {
	if v <= 0 {
		return fallback
	}
	return v
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
