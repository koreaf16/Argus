package tools

import (
	"context"
	"encoding/json"

	"github.com/koreaf16/argus/internal/types"
)

type ToolInputJSONSchema map[string]any

type ToolEventKind string

const (
	ToolEventOutput             ToolEventKind = "output"
	ToolEventChunk              ToolEventKind = "chunk" // 실행 중 스트리밍 청크 (display-only, tool_result에 누적 안 됨)
	ToolEventError              ToolEventKind = "error"
	ToolEventDone               ToolEventKind = "done"
	ToolEventPasswordPrompt     ToolEventKind = "password_prompt"
	ToolEventAskUserPrompt      ToolEventKind = "ask_user_prompt"
	ToolEventAskUserBatchPrompt ToolEventKind = "ask_user_batch_prompt"
)

type AskUserOption struct {
	Value       string `json:"value,omitempty"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
}

type AskUserQuestion struct {
	ID          string          `json:"id,omitempty"`
	Header      string          `json:"header,omitempty"`
	Question    string          `json:"question"`
	Type        string          `json:"type,omitempty"`
	Placeholder string          `json:"placeholder,omitempty"`
	Default     string          `json:"default,omitempty"`
	MultiSelect bool            `json:"multi_select,omitempty"`
	Required    bool            `json:"required,omitempty"`
	Preview     string          `json:"preview,omitempty"`
	Options     []AskUserOption `json:"options,omitempty"`
}

type AskUserResponse struct {
	Value    string `json:"value,omitempty"`
	Canceled bool   `json:"canceled,omitempty"`
	Error    string `json:"error,omitempty"`
}

type AskUserBatchPrompt struct {
	Questions []AskUserQuestion `json:"questions"`
}

type AskUserBatchResponse struct {
	AnswersByIndex map[string]string `json:"answers_by_index,omitempty"`
	AnswersByID    map[string]string `json:"answers_by_id,omitempty"`
	Canceled       bool              `json:"canceled,omitempty"`
	Error          string            `json:"error,omitempty"`
}

type ToolEvent struct {
	Kind                 ToolEventKind             `json:"kind"`
	Output               string                    `json:"output,omitempty"`
	Prompt               string                    `json:"prompt,omitempty"`
	Question             *AskUserQuestion          `json:"question,omitempty"`
	AskUserBatchPrompt   *AskUserBatchPrompt       `json:"ask_user_batch_prompt,omitempty"`
	Err                  error                     `json:"-"`
	PasswordResponse     chan string               `json:"-"` // Channel to receive password input from UI
	AskUserResponse      chan AskUserResponse      `json:"-"`
	AskUserBatchResponse chan AskUserBatchResponse `json:"-"`
	InputResponse        chan string               `json:"-"` // Channel to receive raw terminal input from UI
}

type PermissionResult = types.PermissionResult

// VisibleFilter 는 도구가 특정 컨텍스트(예: 타겟 OS)에서 LLM에게 노출될지 여부를 결정하는 인터페이스입니다.
// 이 인터페이스를 구현하지 않는 도구는 기본적으로 항상 노출됩니다.
type VisibleFilter interface {
	IsVisible(ctx Context) bool
}

// StaticSystemPromptContributor is an optional interface for tools whose
// system-prompt guide text never changes across Context values.
// Static guides are batched into a single cache-eligible system block,
// reducing prompt-cache invalidation. Implement this instead of
// SystemPromptContributor for simple, stateless tools.
type StaticSystemPromptContributor interface {
	GetStaticSystemPromptGuide() string
}

// SystemPromptContributor is an optional interface for tools that inject a
// context-dependent section into the system prompt (e.g. the guide changes
// based on workspace count, mode, or model capabilities). Called once per
// LLM turn. Prefer StaticSystemPromptContributor when the guide is static.
type SystemPromptContributor interface {
	GetSystemPromptGuide(ctx Context) string
}

type Tool interface {
	Name() string
	Description(ctx Context) string
	InputSchema() ToolInputJSONSchema
	IsReadOnly() bool
	Call(ctx Context, input json.RawMessage) (<-chan ToolEvent, error)
	CheckPermission(ctx Context, input json.RawMessage) (PermissionResult, error)
	MaxResultSizeChars() int
}

func DefaultAllowPermission() PermissionResult {
	return types.PermissionResult{Behavior: types.BehaviorAllow}
}

func DefaultAskPermission() PermissionResult {
	return types.PermissionResult{Behavior: types.BehaviorAsk}
}

func NewDoneEvent() ToolEvent {
	return ToolEvent{Kind: ToolEventDone}
}

func NewOutputEvent(output string) ToolEvent {
	return ToolEvent{Kind: ToolEventOutput, Output: output}
}

func NewChunkEvent(output string) ToolEvent {
	return ToolEvent{Kind: ToolEventChunk, Output: output}
}

func NewErrorEvent(err error) ToolEvent {
	return ToolEvent{Kind: ToolEventError, Err: err}
}

func NewContext(ctx context.Context) Context {
	return Context{Context: ctx}
}
