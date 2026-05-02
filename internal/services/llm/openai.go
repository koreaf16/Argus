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
	"unicode/utf8"
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
		"messages": toOpenAIMessagesWithSystem(req.System, req.Messages, req.Thinking != nil),
		"stream":   true,
	}
	// Qwen 모델인 경우 도구 호출 안정성을 위해 페널티 파라미터를 0.0으로 고정
	if strings.Contains(strings.ToLower(model), "qwen") {
		payload["presence_penalty"] = 0.0
		payload["frequency_penalty"] = 0.0
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
	appendRequestLog(o.Provider(), model, req)
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
			if delta == "" {
				return
			}
			if req.TraceHook != nil {
				req.TraceHook(TraceEvent{
					Kind:        TraceKindProviderChunk,
					Provider:    o.Provider(),
					Model:       model,
					Raw:         copyRaw([]byte(raw)),
					MappedKind:  EventTextDelta,
					MappedDelta: delta,
				})
			}
			out <- Event{Kind: EventTextDelta, Delta: delta}
		}

		emitThinking := func(raw, delta string) {
			if delta == "" {
				return
			}
			if req.TraceHook != nil {
				req.TraceHook(TraceEvent{
					Kind:        TraceKindProviderChunk,
					Provider:    o.Provider(),
					Model:       model,
					Raw:         copyRaw([]byte(raw)),
					MappedKind:  EventThinkingDelta,
					MappedDelta: delta,
				})
			}
			out <- Event{Kind: EventThinkingDelta, Delta: delta}
		}

		makeEmitter := func(raw string) filterEmitter {
			return &streamEmitter{
				onText:     func(s string) { emitText(raw, s) },
				onThinking: func(s string) { emitThinking(raw, s) },
			}
		}

		feedFilter := func(raw, delta string) {
			filter.feed(delta, makeEmitter(raw))
		}

		flushFilter := func(raw string) {
			filter.flush(makeEmitter(raw))
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
			// Support both reasoning_content (DeepSeek) and reasoning (vLLM/Qwen)
			rContent := choice.Delta.ReasoningContent
			if rContent == "" {
				rContent = choice.Delta.Reasoning
			}

			if rContent != "" {
				if req.TraceHook != nil {
					req.TraceHook(TraceEvent{
						Kind:        TraceKindProviderChunk,
						Provider:    o.Provider(),
						Model:       model,
						Raw:         copyRaw([]byte(payload)),
						MappedKind:  EventThinkingDelta,
						MappedDelta: rContent,
					})
				}
				out <- Event{Kind: EventThinkingDelta, Delta: rContent}
			}
			if choice.Delta.Content != "" {
				feedFilter(payload, choice.Delta.Content)
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
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			Reasoning        string `json:"reasoning"` // vLLM/Qwen style
			ToolCalls        []struct {
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

func toOpenAIMessagesWithSystem(system []SystemBlock, msgs []Message, thinkingEnabled bool) []map[string]any {
	out := toOpenAIMessages(msgs, thinkingEnabled)
	systemText := strings.TrimSpace(systemToText(system))
	if systemText == "" {
		return out
	}
	withSystem := make([]map[string]any, 0, len(out)+1)
	withSystem = append(withSystem, map[string]any{
		"role":    "system",
		"content": systemText,
	})
	withSystem = append(withSystem, out...)
	return withSystem
}

// filterEmitter는 channelTokenFilter가 분리한 본문을 두 가지 sink(text/thinking)로
// 라우팅하기 위한 콜백 인터페이스입니다.
type filterEmitter interface {
	EmitText(s string)
	EmitThinking(s string)
}

// streamEmitter는 Stream() 내부에서 chunk별 raw payload를 클로저에 capture해
// trace 이벤트와 함께 EventTextDelta / EventThinkingDelta를 발행하는 어댑터입니다.
type streamEmitter struct {
	onText     func(s string)
	onThinking func(s string)
}

func (s *streamEmitter) EmitText(t string)     { s.onText(t) }
func (s *streamEmitter) EmitThinking(t string) { s.onThinking(t) }

type filterState int

const (
	stateOutside       filterState = iota // 일반 텍스트 모드
	stateChannelLabel                     // <|channel|> 직후 라벨 단어 대기
	stateChannelHeader                    // 라벨 결정 후 <|message|> 또는 종료 토큰 대기
	stateChannelBody                      // <|message|> 이후 본문 (sink는 bodySink)
	stateThinkBody                        // <think>...</think> 본문
)

type sinkMode int

const (
	sinkText sinkMode = iota
	sinkThinking
)

// channelTokenFilter는 OpenAI Harmony / Gemma / qwen 등 일부 모델이 응답 본문에 흘려보내는
// 다음 형식들을 인식해 본문을 text/thinking 두 sink로 분리해 emit합니다.
//
//   - <|channel|>(analysis|thought|commentary)<|message|>...<|end|>  → thinking sink
//   - <|channel|>final<|message|>...<|return|>                       → text sink
//   - <think>...</think>                                             → thinking sink
//   - <|channel>label\n / <channel|> / <|message|> 등 단발 토큰        → 무시(outside 유지)
//
// chunk 경계에서 토큰이 잘리는 경우 pendingChunk 버퍼로 hold하여 다음 chunk와 합쳐 재처리합니다.
type channelTokenFilter struct {
	state        filterState
	bodySink     sinkMode
	pendingChunk string // chunk 경계에서 잘린 미완성 토큰/라벨
	pendingLine  string // outside 모드에서 standalone 라벨 라인 검사용 버퍼
}

// 채널 라벨 → sink 결정 (대소문자 구분 없음, lowercase 비교).
var harmonyLabelSink = map[string]sinkMode{
	"analysis":   sinkThinking,
	"thought":    sinkThinking,
	"commentary": sinkThinking,
	"final":      sinkText,
}

// 단독으로 한 줄에 등장하면 outside text sink에서 제거할 Harmony 채널 라벨.
var harmonyStandaloneLabel = map[string]bool{
	"analysis":   true,
	"thought":    true,
	"commentary": true,
	"final":      true,
}

// chunk 경계에서 부분 토큰일 가능성을 판단할 때 사용하는 알려진 키워드 목록.
var harmonyTokenKeywords = []string{
	"channel", "message", "end", "return", "start", "stop", "done",
	"assistant", "user", "system", "developer", "tool", "think",
}

// outside 모드에서 단발 제거 대상이 되는 제어 토큰들 (라벨 없음, 그냥 사라짐).
// tool_call 변형들은 native tool calling을 지원 못하는 provider가 inline pseudo-syntax로
// 흘려보내는 패턴 — 그대로 두면 사용자 화면에 raw 노출되므로 단발 제거한다.
var harmonyControlTokens = []string{
	"<|message|>", "<|end|>", "<|return|>", "<|start|>", "<|stop|>", "<|done|>",
	"<|assistant|>", "<|user|>", "<|system|>", "<|developer|>", "<|tool|>",
	"<channel|>",
	"<|tool_call|>", "<tool_call|>", "<tool_call>", "</tool_call>",
}

const (
	tokenChannelOpen = "<|channel|>"
	tokenChannelLeft = "<|channel>" // 좌측 단일 파이프 변형 (라벨이 라인 형태로 따라옴)
	tokenMessage     = "<|message|>"
	tokenEnd         = "<|end|>"
	tokenReturn      = "<|return|>"
	tokenThinkOpen   = "<think>"
	tokenThinkClose  = "</think>"
)

func (f *channelTokenFilter) feed(delta string, e filterEmitter) {
	text := f.pendingChunk + delta
	f.pendingChunk = ""

	for len(text) > 0 {
		var consumed int
		var hold bool
		switch f.state {
		case stateOutside:
			consumed, hold = f.feedOutside(text, e)
		case stateChannelLabel:
			consumed, hold = f.feedChannelLabel(text)
		case stateChannelHeader:
			consumed, hold = f.feedChannelHeader(text)
		case stateChannelBody:
			consumed, hold = f.feedChannelBody(text, e)
		case stateThinkBody:
			consumed, hold = f.feedThinkBody(text, e)
		}
		if hold {
			f.pendingChunk = text[consumed:]
			return
		}
		if consumed == 0 {
			// 진행 보장 (안전망 — 정상 경로에선 발생하지 않아야 함).
			return
		}
		text = text[consumed:]
	}
}

func (f *channelTokenFilter) flush(e filterEmitter) {
	// 미완성 토큰은 폐기. 단, thinking/think 본문 모드에서 끊긴 경우 적절한 sink로 흘려 데이터 손실 방지.
	if len(f.pendingChunk) > 0 {
		switch f.state {
		case stateChannelBody:
			f.emitBody(f.pendingChunk, e)
		case stateThinkBody:
			e.EmitThinking(f.pendingChunk)
		case stateOutside:
			f.appendOutsideText(f.pendingChunk, e)
		}
		f.pendingChunk = ""
	}

	line := f.pendingLine
	f.pendingLine = ""
	if line == "" {
		return
	}
	if harmonyStandaloneLabel[strings.TrimSpace(line)] {
		return
	}
	if f.state == stateChannelBody {
		f.emitBody(line, e)
		return
	}
	if f.state == stateThinkBody {
		e.EmitThinking(line)
		return
	}
	e.EmitText(line)
}

// feedOutside는 outside 모드에서 가장 빨리 등장하는 토큰을 찾아 처리합니다.
// 토큰 이전 본문은 라인 버퍼에 누적해 standalone 라벨 검사 후 EmitText로 흘립니다.
func (f *channelTokenFilter) feedOutside(text string, e filterEmitter) (consumed int, hold bool) {
	pos, kind, length := f.findOutsideToken(text)
	if pos == -1 {
		safe, held := splitAtPotentialToken(text)
		if held == "" {
			safe, held = splitAtTrailingPartialUTF8(text)
		}
		f.appendOutsideText(safe, e)
		if held != "" {
			return len(safe), true
		}
		return len(text), false
	}
	f.appendOutsideText(text[:pos], e)
	consumed = pos + length
	switch kind {
	case outsideTokChannelOpen:
		f.flushOutsideLine(e)
		f.state = stateChannelLabel
	case outsideTokThinkOpen:
		f.flushOutsideLine(e)
		f.state = stateThinkBody
	case outsideTokChannelLeft:
		// 이미 length에 포함됨
	case outsideTokControl:
	}
	return consumed, false
}

type outsideTokenKind int

const (
	outsideTokChannelOpen outsideTokenKind = iota
	outsideTokThinkOpen
	outsideTokChannelLeft
	outsideTokControl
)

// findOutsideToken은 text 안에서 outside 모드의 가장 빠른 토큰 위치를 반환합니다.
// pos == -1이면 토큰 없음.
func (f *channelTokenFilter) findOutsideToken(text string) (pos int, kind outsideTokenKind, length int) {
	pos = -1
	consider := func(p, l int, k outsideTokenKind) {
		if p < 0 {
			return
		}
		if pos == -1 || p < pos {
			pos = p
			kind = k
			length = l
		}
	}
	consider(strings.Index(text, tokenChannelOpen), len(tokenChannelOpen), outsideTokChannelOpen)
	consider(strings.Index(text, tokenThinkOpen), len(tokenThinkOpen), outsideTokThinkOpen)
	if p := strings.Index(text, tokenChannelLeft); p >= 0 && pos != p {
		// 좌측 단일 파이프: <|channel>label\n 형태. \n까지 통째 소거.
		end := p + len(tokenChannelLeft)
		nl := strings.IndexByte(text[end:], '\n')
		if nl == -1 {
			// 라인 종료 없음 — 다음 chunk와 합쳐 재처리.
			if pos == -1 || p < pos {
				return p, outsideTokChannelLeft, len(text) - p
			}
		} else if pos == -1 || p < pos {
			pos = p
			kind = outsideTokChannelLeft
			length = (end + nl + 1) - p
		}
	}
	for _, t := range harmonyControlTokens {
		consider(strings.Index(text, t), len(t), outsideTokControl)
	}
	return pos, kind, length
}

// appendOutsideText는 outside text를 라인 버퍼에 누적하고, 완성된 라인은 standalone 라벨 검사를 거쳐 EmitText로 흘립니다.
func (f *channelTokenFilter) appendOutsideText(s string, e filterEmitter) {
	if s == "" {
		return
	}
	combined := f.pendingLine + s
	lastNL := strings.LastIndex(combined, "\n")
	if lastNL == -1 {
		f.pendingLine = combined
		return
	}
	complete := combined[:lastNL+1]
	f.pendingLine = combined[lastNL+1:]
	if filtered := removeStandaloneLabelLines(complete); filtered != "" {
		e.EmitText(filtered)
	}
}

// flushOutsideLine은 라인 버퍼에 남은 미완성 라인을 EmitText로 흘립니다 (채널/think 진입 시 호출).
func (f *channelTokenFilter) flushOutsideLine(e filterEmitter) {
	if f.pendingLine == "" {
		return
	}
	line := f.pendingLine
	f.pendingLine = ""
	if harmonyStandaloneLabel[strings.TrimSpace(line)] {
		return
	}
	e.EmitText(line)
}

func (f *channelTokenFilter) feedChannelLabel(text string) (consumed int, hold bool) {
	// 라벨 단어 식별 (lowercase 영문자). 비-영문자 만나면 라벨 종료.
	end := 0
	for end < len(text) {
		c := text[end]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
			end++
			continue
		}
		break
	}
	if end == 0 {
		// 라벨 없음 — 토큰 정상 형식 아님. outside 복귀.
		f.state = stateOutside
		return 0, false
	}
	if end == len(text) {
		// chunk 끝까지 라벨 — 부분일 수 있음. hold.
		return 0, true
	}
	label := strings.ToLower(text[:end])
	if sink, ok := harmonyLabelSink[label]; ok {
		f.bodySink = sink
	} else {
		// 미지의 라벨 — 안전상 thinking으로 라우팅 (그러나 노출은 안 됨).
		f.bodySink = sinkThinking
	}
	f.state = stateChannelHeader
	return end, false
}

func (f *channelTokenFilter) feedChannelHeader(text string) (consumed int, hold bool) {
	// 가장 빠른 헤더 종결 토큰 찾기.
	pos, length, transition := f.findHeaderToken(text)
	if pos == -1 {
		// 헤더 영역 잡음 — 부분 토큰일 수 있는지 확인.
		safe, held := splitAtPotentialToken(text)
		_ = safe // 헤더 영역 텍스트는 무시.
		if held != "" {
			return len(text) - len(held), true
		}
		return len(text), false
	}
	// 토큰 이전 텍스트는 헤더 잡음으로 무시.
	consumed = pos + length
	f.state = transition
	return consumed, false
}

// findHeaderToken은 channelHeader 모드에서 인식할 토큰을 찾습니다.
func (f *channelTokenFilter) findHeaderToken(text string) (pos, length int, transition filterState) {
	pos = -1
	consider := func(p, l int, t filterState) {
		if p < 0 {
			return
		}
		if pos == -1 || p < pos {
			pos = p
			length = l
			transition = t
		}
	}
	consider(strings.Index(text, tokenMessage), len(tokenMessage), stateChannelBody)
	consider(strings.Index(text, tokenEnd), len(tokenEnd), stateOutside)
	consider(strings.Index(text, tokenReturn), len(tokenReturn), stateOutside)
	consider(strings.Index(text, tokenChannelOpen), len(tokenChannelOpen), stateChannelLabel)
	return pos, length, transition
}

func (f *channelTokenFilter) feedChannelBody(text string, e filterEmitter) (consumed int, hold bool) {
	pos, length, transition := f.findBodyTerminator(text)
	if pos == -1 {
		safe, held := splitAtPotentialToken(text)
		if held == "" {
			safe, held = splitAtTrailingPartialUTF8(text)
		}
		f.emitBody(safe, e)
		if held != "" {
			return len(safe), true
		}
		return len(text), false
	}
	f.emitBody(text[:pos], e)
	consumed = pos + length
	f.state = transition
	return consumed, false
}

// findBodyTerminator는 channelBody 모드의 종료 토큰을 찾습니다.
func (f *channelTokenFilter) findBodyTerminator(text string) (pos, length int, transition filterState) {
	pos = -1
	consider := func(p, l int, t filterState) {
		if p < 0 {
			return
		}
		if pos == -1 || p < pos {
			pos = p
			length = l
			transition = t
		}
	}
	consider(strings.Index(text, tokenEnd), len(tokenEnd), stateOutside)
	consider(strings.Index(text, tokenReturn), len(tokenReturn), stateOutside)
	consider(strings.Index(text, tokenChannelOpen), len(tokenChannelOpen), stateChannelLabel)
	return pos, length, transition
}

func (f *channelTokenFilter) emitBody(s string, e filterEmitter) {
	if s == "" {
		return
	}
	if f.bodySink == sinkThinking {
		e.EmitThinking(s)
		return
	}
	e.EmitText(s)
}

func (f *channelTokenFilter) feedThinkBody(text string, e filterEmitter) (consumed int, hold bool) {
	idx := strings.Index(text, tokenThinkClose)
	if idx == -1 {
		// 닫기 태그 없음. 끝부분이 </think 부분 매칭일 수 있는지 확인.
		safe, held := splitAtPotentialThinkClose(text)
		if held == "" {
			safe, held = splitAtTrailingPartialUTF8(text)
		}
		if safe != "" {
			e.EmitThinking(safe)
		}
		if held != "" {
			return len(safe), true
		}
		return len(text), false
	}
	e.EmitThinking(text[:idx])
	consumed = idx + len(tokenThinkClose)
	f.state = stateOutside
	return consumed, false
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

// splitAtTrailingPartialUTF8은 chunk 끝부분이 미완성 UTF-8 멀티바이트 시퀀스인 경우 hold합니다.
func splitAtTrailingPartialUTF8(s string) (safe, held string) {
	for i := 1; i <= 3 && i <= len(s); i++ {
		sub := s[len(s)-i:]
		if utf8.RuneStart(sub[0]) {
			if !utf8.FullRuneInString(sub) {
				return s[:len(s)-i], sub
			}
			return s, ""
		}
	}
	return s, ""
}

// splitAtPotentialThinkClose는 thinkBody 모드 청크 끝이 </think 부분 매칭일 때 hold합니다.
func splitAtPotentialThinkClose(text string) (safe, held string) {
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
	if isPartialThinkClose(suffix) {
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
	// </think 부분 매칭도 hold 대상.
	if rest[0] == '/' {
		return isPartialThinkCloseRest(rest[1:])
	}
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

func isPartialThinkClose(s string) bool {
	if len(s) == 0 || s[0] != '<' {
		return false
	}
	if len(s) == 1 {
		return true
	}
	if s[1] != '/' {
		return false
	}
	return isPartialThinkCloseRest(s[2:])
}

func isPartialThinkCloseRest(rest string) bool {
	// rest는 "think" 까지의 부분 매칭이어야 함.
	const target = "think"
	if len(rest) > len(target) {
		return false
	}
	return strings.HasPrefix(target, rest)
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
