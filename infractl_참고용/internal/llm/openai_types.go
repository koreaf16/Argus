// Package llm
// File: openai_types.go
// Description: OpenAI 호환 API 요청/응답의 내부 JSON 구조체 정의
// Responsibility: OpenAI API HTTP 통신에 사용되는 내부 타입 정의

package llm

// chatRequest는 OpenAI 호환 API에 전송하는 요청 본문이다.
type chatRequest struct {
	Model       string      `json:"model"`
	Messages    []Message   `json:"messages"`
	Tools       []ToolDef   `json:"tools,omitempty"`       // 비어있으면 필드 자체를 제외
	ToolChoice  interface{} `json:"tool_choice,omitempty"`
	Stream      bool        `json:"stream"`
	Temperature *float64    `json:"temperature,omitempty"` // nil이면 서버 기본값(1.0) 사용
	MaxTokens   *int        `json:"max_tokens,omitempty"`  // nil이면 모델 기본값 사용
}

// streamChunk는 SSE 스트리밍 응답의 단일 청크를 나타낸다.
type streamChunk struct {
	Choices []streamChoice `json:"choices"`
	Usage   *usageInfo     `json:"usage"`
}

// streamChoice는 스트리밍 응답의 단일 delta를 나타낸다.
type streamChoice struct {
	Delta        streamDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

// streamDelta는 스트리밍 응답의 증분 데이터를 나타낸다.
type streamDelta struct {
	Reasoning string           `json:"reasoning"`  // vLLM --reasoning-parser 전용
	Content   string           `json:"content"`
	ToolCalls []streamToolCall `json:"tool_calls"`
}

// streamToolCall은 스트리밍 응답에서 tool_calls의 단일 delta를 나타낸다.
// 여러 청크에 걸쳐 누적된다.
type streamToolCall struct {
	Index    int                  `json:"index"`
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function streamFunctionDelta  `json:"function"`
}

// streamFunctionDelta는 스트리밍으로 전달되는 함수 호출의 증분 데이터이다.
type streamFunctionDelta struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // 여러 청크에 걸쳐 이어붙임
}

// usageInfo는 API 토큰 사용량을 나타낸다.
type usageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}
