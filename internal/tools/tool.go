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
