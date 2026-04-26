// Package llm
// File: types.go
// Description: LLM API 통신에 사용되는 메시지, 응답, 도구 정의 타입
// Responsibility: LLM 통신 관련 데이터 타입 정의

package llm

// Role은 대화 메시지의 역할을 나타낸다.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message는 LLM API의 단일 대화 메시지를 나타낸다.
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // assistant role에서만 사용
	ToolCallID string     `json:"tool_call_id,omitempty"` // tool role에서만 사용
}

// ToolCall은 LLM이 요청한 단일 도구 호출을 나타낸다.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // 항상 "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall은 호출할 함수명과 JSON 인자를 나타낸다.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON 문자열
}

// Response는 LLM API 호출의 최종 응답 결과를 나타낸다.
type Response struct {
	Content      string
	Thinking     string     // vLLM --reasoning-parser의 reasoning 필드 내용
	ToolCalls    []ToolCall
	InputTokens  int
	OutputTokens int
}

// ToolDef는 LLM에 전달하는 도구 정의를 나타낸다.
// OpenAI function calling 스키마 형식을 따른다.
type ToolDef struct {
	Type     string      `json:"type"` // 항상 "function"
	Function FunctionDef `json:"function"`
}

// FunctionDef는 도구의 이름, 설명, 파라미터 JSON Schema를 나타낸다.
type FunctionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}
