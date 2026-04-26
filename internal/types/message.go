// Package types — LLM 대화 메시지 및 콘텐츠 블록 타입.
//
// 파일 역할: Anthropic Messages API 의 메시지 구조를 Go 로 정의한다.
//
//	UserMessage, AssistantMessage, ToolUse, ToolResult 등
//	LLM 대화의 모든 메시지 타입과 콘텐츠 블록 타입을 포함한다.
//
// 포함 모듈:
//   - ContentBlockKind: 콘텐츠 블록 종류 열거형.
//   - ContentBlock: 콘텐츠 블록 유니언 타입 (TextBlock / ToolUseBlock / ToolResultBlock 등).
//   - UserMessage / AssistantMessage: 역할별 메시지.
//   - Message: 전체 메시지 유니언 타입.
//   - StopReason: 스트림 종료 이유.
//
// 호출/사용 방식:
//   - internal/query/engine.go, internal/services/llm 등에서 메시지 조립·파싱 시 참조.
//
// 연결:
//   - 원본: src/utils/messages.ts 및 Anthropic API 타입에서 도출.
package types

import "encoding/json"

// MessageRole 은 메시지 발신자 역할이다.
type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
)

// ContentBlockKind 는 콘텐츠 블록 종류다.
type ContentBlockKind string

const (
	BlockKindText       ContentBlockKind = "text"
	BlockKindThinking   ContentBlockKind = "thinking"
	BlockKindToolUse    ContentBlockKind = "tool_use"
	BlockKindToolResult ContentBlockKind = "tool_result"
	BlockKindImage      ContentBlockKind = "image"
	BlockKindDocument   ContentBlockKind = "document"
)

// TextBlock 은 일반 텍스트 콘텐츠 블록이다.
type TextBlock struct {
	Type ContentBlockKind `json:"type"` // "text"
	Text string           `json:"text"`
}

// ThinkingBlock 은 모델의 내부 추론 블록이다.
type ThinkingBlock struct {
	Type      ContentBlockKind `json:"type"` // "thinking"
	Thinking  string           `json:"thinking"`
	Signature string           `json:"signature,omitempty"`
}

// ToolUseBlock 은 모델이 도구를 호출할 때의 블록이다.
type ToolUseBlock struct {
	Type  ContentBlockKind `json:"type"` // "tool_use"
	ID    string           `json:"id"`
	Name  string           `json:"name"`
	Input json.RawMessage  `json:"input"`
}

// ToolResultContent 는 tool_result 블록 안에 올 수 있는 콘텐츠다.
type ToolResultContent struct {
	Type ContentBlockKind `json:"type"` // "text" | "image"
	Text string           `json:"text,omitempty"`
}

// ToolResultBlock 은 도구 실행 결과를 담는 블록이다.
type ToolResultBlock struct {
	Type      ContentBlockKind    `json:"type"` // "tool_result"
	ToolUseID string              `json:"tool_use_id"`
	Content   []ToolResultContent `json:"content,omitempty"`
	IsError   bool                `json:"is_error,omitempty"`
}

// ImageSource 는 이미지 블록의 데이터 소스다.
type ImageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "image/png" 등
	Data      string `json:"data"`
}

// ImageBlock 은 이미지 콘텐츠 블록이다.
type ImageBlock struct {
	Type   ContentBlockKind `json:"type"` // "image"
	Source ImageSource      `json:"source"`
}

// ContentBlock 은 메시지 안에 들어갈 수 있는 모든 블록의 유니언이다.
// Kind() 로 실제 타입을 판별한 뒤 해당 필드를 참조한다.
type ContentBlock struct {
	kind ContentBlockKind

	Text       *TextBlock
	Thinking   *ThinkingBlock
	ToolUse    *ToolUseBlock
	ToolResult *ToolResultBlock
	Image      *ImageBlock
}

// NewTextBlock 은 텍스트 블록 ContentBlock 을 생성한다.
func NewTextBlock(text string) ContentBlock {
	return ContentBlock{kind: BlockKindText, Text: &TextBlock{Type: BlockKindText, Text: text}}
}

// NewThinkingBlock 은 thinking 블록 ContentBlock 을 생성한다.
func NewThinkingBlock(thinking, signature string) ContentBlock {
	return ContentBlock{kind: BlockKindThinking, Thinking: &ThinkingBlock{Type: BlockKindThinking, Thinking: thinking, Signature: signature}}
}

// NewToolUseBlock 은 tool_use 블록 ContentBlock 을 생성한다.
func NewToolUseBlock(id, name string, input json.RawMessage) ContentBlock {
	return ContentBlock{kind: BlockKindToolUse, ToolUse: &ToolUseBlock{Type: BlockKindToolUse, ID: id, Name: name, Input: input}}
}

// NewToolResultBlock 은 tool_result 블록 ContentBlock 을 생성한다.
func NewToolResultBlock(toolUseID, text string, isError bool) ContentBlock {
	content := []ToolResultContent{{Type: BlockKindText, Text: text}}
	return ContentBlock{kind: BlockKindToolResult, ToolResult: &ToolResultBlock{
		Type:      BlockKindToolResult,
		ToolUseID: toolUseID,
		Content:   content,
		IsError:   isError,
	}}
}

// Kind 는 이 블록의 종류를 반환한다.
func (b ContentBlock) Kind() ContentBlockKind { return b.kind }

// StopReason 은 LLM 스트림이 종료된 이유다.
type StopReason string

const (
	StopReasonEndTurn      StopReason = "end_turn"
	StopReasonToolUse      StopReason = "tool_use"
	StopReasonMaxTokens    StopReason = "max_tokens"
	StopReasonStopSequence StopReason = "stop_sequence"
)

// UserMessage 는 사용자(human) 역할의 메시지다.
type UserMessage struct {
	Role    MessageRole    `json:"role"` // "user"
	Content []ContentBlock `json:"content"`
}

// AssistantMessage 는 어시스턴트 역할의 메시지다.
type AssistantMessage struct {
	Role    MessageRole    `json:"role"` // "assistant"
	Content []ContentBlock `json:"content"`
}

// Message 는 대화에서 오가는 모든 메시지를 표현한다.
type Message struct {
	Role    MessageRole    `json:"role"`
	Content []ContentBlock `json:"content"`
}

// NewUserMessage 는 텍스트 하나짜리 UserMessage 를 만든다.
func NewUserMessage(text string) Message {
	return Message{Role: RoleUser, Content: []ContentBlock{NewTextBlock(text)}}
}

// NewAssistantMessage 는 콘텐츠 블록 목록으로 AssistantMessage 를 만든다.
func NewAssistantMessage(blocks []ContentBlock) Message {
	return Message{Role: RoleAssistant, Content: blocks}
}

// MessageOrigin 은 메시지가 발생한 출처를 나타낸다.
type MessageOrigin string

const (
	OriginUser      MessageOrigin = "user"
	OriginSystem    MessageOrigin = "system"
	OriginTool      MessageOrigin = "tool"
	OriginScheduled MessageOrigin = "scheduled"
)

// Usage 는 API 응답의 토큰 사용량이다.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
}
