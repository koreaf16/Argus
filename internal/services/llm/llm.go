// 파일 역할: LLM 서비스의 기본 인터페이스 및 공통 데이터 구조 정의.
// 포함 모듈: context, encoding/json 등.
// 호출/사용 방식: LLM 공급자(Anthropic, Gemini, OpenAI) 구현의 기반 인터페이스.
// 연결: internal/services/llm 하위 구현체들의 인터페이스 정의.

package llm

import (
	"context"
	"encoding/json"
	"errors"
)

type Provider string

const (
	ProviderAnthropic    Provider = "anthropic"
	ProviderGemini       Provider = "gemini"
	ProviderOpenAICompat Provider = "openai-compat"
)

type Caps struct {
	Tools     bool `json:"tools"`
	Thinking  bool `json:"thinking"`
	Vision    bool `json:"vision"`
	WebSearch bool `json:"web_search"`
}

type LLM interface {
	Stream(ctx context.Context, req Request) (<-chan Event, error)
	CountTokens(ctx context.Context, req Request) (int, error)
	Capabilities() Caps
	Provider() string
}

type ThinkingConfig struct {
	Type         string `json:"type,omitempty"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

type SystemBlock struct {
	Type         string         `json:"type"`
	Text         string         `json:"text,omitempty"`
	CacheControl map[string]any `json:"cache_control,omitempty"`
}

type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
)

type ContentType string

const (
	ContentText       ContentType = "text"
	ContentThinking   ContentType = "thinking"
	ContentToolUse    ContentType = "tool_use"
	ContentToolResult ContentType = "tool_result"
)

type ContentBlock struct {
	Type      ContentType     `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type Message struct {
	Role    MessageRole    `json:"role"`
	Content []ContentBlock `json:"content"`
}

func TextMessage(role MessageRole, text string) Message {
	return Message{
		Role: role,
		Content: []ContentBlock{
			{
				Type: ContentText,
				Text: text,
			},
		},
	}
}

type ToolSpec struct {
	Type        string         `json:"type,omitempty"`
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

type Request struct {
	Model             string          `json:"model"`
	System            []SystemBlock   `json:"system,omitempty"`
	Messages          []Message       `json:"messages"`
	Tools             []ToolSpec      `json:"tools,omitempty"`
	MaxTokens         int             `json:"max_tokens,omitempty"`
	Temperature       *float64        `json:"temperature,omitempty"`
	TopP              *float64        `json:"top_p,omitempty"`
	TopK              *int            `json:"top_k,omitempty"`
	MinP              *float64        `json:"min_p,omitempty"`
	PresencePenalty   *float64        `json:"presence_penalty,omitempty"`
	FrequencyPenalty  *float64        `json:"frequency_penalty,omitempty"`
	RepetitionPenalty *float64        `json:"repetition_penalty,omitempty"`
	Thinking          *ThinkingConfig `json:"thinking,omitempty"`
	TraceHook         TraceHook       `json:"-"`
}

type TraceKind string

const (
	TraceKindRequest       TraceKind = "request"
	TraceKindProviderChunk TraceKind = "provider_chunk"
)

type TraceEvent struct {
	Kind           TraceKind       `json:"kind"`
	Provider       string          `json:"provider,omitempty"`
	Model          string          `json:"model,omitempty"`
	Raw            json.RawMessage `json:"raw,omitempty"`
	Request        *Request        `json:"request,omitempty"`
	MappedKind     EventKind       `json:"mapped_kind,omitempty"`
	MappedDelta    string          `json:"mapped_delta,omitempty"`
	MappedToolName string          `json:"mapped_tool_name,omitempty"`
	MappedStop     *StopReason     `json:"mapped_stop,omitempty"`
}

type TraceHook func(event TraceEvent)

type EventKind string

const (
	EventTextDelta     EventKind = "text_delta"
	EventThinkingDelta EventKind = "thinking_delta"
	EventToolUseStart  EventKind = "tool_use_start"
	EventToolUseDelta  EventKind = "tool_use_delta"
	EventStop          EventKind = "stop"
	EventError         EventKind = "error"
	EventUsage         EventKind = "usage"
)

// UsageStats holds token counts from an LLM API response.
// CacheCreationInputTokens and CacheReadInputTokens are Anthropic-specific
// and indicate prompt-cache write vs. read on each turn.
type UsageStats struct {
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
}

type StopReason string

const (
	StopReasonEndTurn   StopReason = "end_turn"
	StopReasonToolUse   StopReason = "tool_use"
	StopReasonMaxTokens StopReason = "max_tokens"
	StopReasonUnknown   StopReason = "unknown"
)

type ToolUseStart struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type Event struct {
	Kind    EventKind     `json:"kind"`
	Delta   string        `json:"delta,omitempty"`
	ToolUse *ToolUseStart `json:"tool_use,omitempty"`
	Stop    *StopReason   `json:"stop,omitempty"`
	Err     error         `json:"-"`
	Usage   *UsageStats   `json:"usage,omitempty"`
}

type ErrorLLM struct {
	provider string
	caps     Caps
	err      error
}

func NewErrorLLM(provider string, caps Caps, err error) *ErrorLLM {
	if err == nil {
		err = errors.New("llm is not available")
	}
	return &ErrorLLM{
		provider: provider,
		caps:     caps,
		err:      err,
	}
}

func (e *ErrorLLM) Stream(ctx context.Context, req Request) (<-chan Event, error) {
	_ = ctx
	_ = req
	return nil, e.err
}

func (e *ErrorLLM) CountTokens(ctx context.Context, req Request) (int, error) {
	_ = ctx
	return ApproximateTokenCount(req), nil
}

func (e *ErrorLLM) Capabilities() Caps { return e.caps }

func (e *ErrorLLM) Provider() string { return e.provider }

// ApproximateTokenCount 는 Request 의 추정 토큰 수를 반환한다.
// CJK(한글·한자·가나) 문자를 별도로 계산해 단순 chars/4보다 정확하다.
func ApproximateTokenCount(req Request) int {
	total := 0
	for _, s := range req.System {
		total += approximateTextTokens(s.Text)
	}
	for _, m := range req.Messages {
		total += 4 // role/content framing overhead
		total += approximateTextTokens(string(m.Role))
		for _, b := range m.Content {
			total += approximateTextTokens(b.Text)
			total += approximateTextTokens(b.ID)
			total += approximateTextTokens(b.Name)
			total += approximateTextTokens(b.ToolUseID)
			total += approximateJSONTokens(b.Input)
			if b.IsError {
				total++
			}
		}
	}
	for _, t := range req.Tools {
		total += 8 // function declaration framing overhead
		total += approximateTextTokens(t.Name)
		total += approximateTextTokens(t.Description)
		total += approximateJSONTokens(t.InputSchema)
	}
	if total == 0 {
		return 0
	}
	return total
}

// approximateTextTokens 는 텍스트의 추정 토큰 수를 반환한다.
// ASCII: 4 chars/token, CJK(한글·한자 등): 1.5 chars/token.
func approximateTextTokens(text string) int {
	if text == "" {
		return 0
	}
	var ascii, cjk int
	for _, r := range text {
		if r < 0x80 {
			ascii++
		} else if isCJKRune(r) {
			cjk++
		} else {
			ascii++
		}
	}
	tokens := ascii/4 + (cjk*2+1)/3
	if tokens == 0 && (ascii > 0 || cjk > 0) {
		return 1
	}
	return tokens
}

func approximateJSONTokens(v any) int {
	if v == nil {
		return 0
	}
	switch raw := v.(type) {
	case json.RawMessage:
		if len(raw) == 0 {
			return 0
		}
		return approximateTextTokens(string(raw))
	}
	payload, err := json.Marshal(v)
	if err != nil || len(payload) == 0 || string(payload) == "null" {
		return 0
	}
	return approximateTextTokens(string(payload))
}

func isCJKRune(r rune) bool {
	return (r >= 0xAC00 && r <= 0xD7A3) || // 한글 음절
		(r >= 0x1100 && r <= 0x11FF) || // 한글 자모
		(r >= 0x3040 && r <= 0x30FF) || // 히라가나·가타카나
		(r >= 0x4E00 && r <= 0x9FFF) || // CJK 통합 한자
		(r >= 0xF900 && r <= 0xFAFF) // CJK 호환 한자
}
