// Package llm
// File: client.go
// Description: LLM API 클라이언트 인터페이스 정의
// Responsibility: LLM 통신 추상화 인터페이스 제공

package llm

import "context"

// CallOptions는 LLM API 호출 시 추가적인 옵션을 지정하기 위한 구조체이다.
type CallOptions struct {
	MaxTokens *int
}

// CallOption은 CallOptions를 수정하는 함수 타입이다.
type CallOption func(*CallOptions)

// WithMaxTokens는 max_tokens 파라미터를 오버라이드하는 옵션을 반환한다.
func WithMaxTokens(maxTokens int) CallOption {
	return func(o *CallOptions) {
		o.MaxTokens = &maxTokens
	}
}

// Client는 LLM API와 통신하는 인터페이스이다.
// 구현체: OpenAIClient (OpenAI 호환 API용)
type Client interface {
	// Chat은 단일 요청-응답(Non-streaming)을 수행한다.
	// toolChoice: 특정 도구 호출을 강제할 때 사용 (예: "auto", "none", 또는 {"type":"function", "function":{"name":"..."}})
	Chat(ctx context.Context, messages []Message, tools []ToolDef, toolChoice interface{}, opts ...CallOption) (Response, error)

	// ChatStream은 스트리밍 방식으로 LLM에 메시지를 전송한다.
	// toolChoice는 강제 툴 호출 옵션이다 (nil이면 기본 auto).
	// onThinkingToken은 </think> 이전의 추론 토큰에 호출된다 (nil 허용).
	// onToken은 </think> 이후의 최종 응답 토큰에 호출된다 (nil 허용).
	// tool_calls가 포함된 경우 최종 Response에 조합하여 반환한다.
	ChatStream(ctx context.Context, messages []Message, tools []ToolDef, toolChoice interface{}, onThinkingToken func(string), onToken func(string), opts ...CallOption) (Response, error)
}

// InlineToolCallModeClient는 inline tool call 모드를 복제/전환할 수 있는 선택적 확장 인터페이스다.
// 분류 단계처럼 API function calling이 반드시 필요한 경로에서만 사용할 수 있다.
type InlineToolCallModeClient interface {
	Client
	WithInlineToolCalls(enabled bool) Client
	IsInlineToolCalls() bool
}
