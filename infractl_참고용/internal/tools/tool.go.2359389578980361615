package tools

import (
	"context"

	"github.com/yourorg/infractl/internal/executor"
)

// Tool is the common interface exposed to the agent runtime.
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]interface{}
	IsReadOnly() bool
	IsEnabled() bool
	Execute(ctx context.Context, args map[string]interface{}, exec executor.Executor) (ToolOutcome, error)
}

// ConcurrencySafeTool can provide argument-aware concurrency safety checks.
// Tools that do not implement this fall back to IsReadOnly() in partitioning.
type ConcurrencySafeTool interface {
	IsConcurrencySafe(args map[string]interface{}) bool
}

// ToolOutcome captures richer execution results for tools that can classify
// operational success independently from transport/runtime errors.
type ToolOutcome struct {
	Content      string
	Success      bool
	ExitCode     int
	ErrorMessage string
	MetadataJSON string
}

// ModelPromptDescriber optionally provides a model-facing concise description.
type ModelPromptDescriber interface {
	ModelPrompt() string
}

// DisplayDescriber optionally provides a UI-facing description.
type DisplayDescriber interface {
	DisplayDescription() string
}

// ResultSummarizer optionally provides a compact summary of tool output.
type ResultSummarizer interface {
	ResultSummary(output string) string
}

// ExecuteWithOutcome은 ToolOutcome을 반환하는 도구 실행을 돕는 헬퍼다.
func ExecuteWithOutcome(ctx context.Context, tool Tool, args map[string]interface{}, exec executor.Executor) (ToolOutcome, error) {
	return tool.Execute(ctx, args, exec)
}

// QuestionOption is one selectable answer.
type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// QuestionRequest describes a user prompt shown by the UI.
type QuestionRequest struct {
	Question         string           `json:"question"`
	Options          []QuestionOption `json:"options"`
	AllowCustomInput bool             `json:"allow_custom_input,omitempty"`
	// Header is the optional title shown above the question UI.
	// Leave it empty for generic prompts with no visible title.
	// Use explicit headers for confirmations or security prompts.
	Header string `json:"header,omitempty"`
}

// QuestionResponse is the user's answer.
type QuestionResponse struct {
	SelectedIndex int    `json:"selected_index"`
	SelectedLabel string `json:"selected_label"`
	UserInput     string `json:"user_input"`
	IsOther       bool   `json:"is_other"`
}

// FormFieldDef는 폼 입력 필드 하나의 정의다.
type FormFieldDef struct {
	Name         string   `json:"name"`
	Label        string   `json:"label"`
	Placeholder  string   `json:"placeholder,omitempty"`
	Required     bool     `json:"required,omitempty"`
	DefaultValue string   `json:"default_value,omitempty"`
	FieldType    string   `json:"field_type,omitempty"` // "text"(기본) | "select"
	Options      []string `json:"options,omitempty"`    // FieldType=="select"일 때 선택지 목록
}

// FormRequest는 에이전트가 TUI에 폼 입력을 요청할 때 사용한다.
type FormRequest struct {
	Title  string         `json:"title"`
	Fields []FormFieldDef `json:"fields"`
	Header string         `json:"header,omitempty"`
}

// FormResponse는 사용자가 폼을 완성하고 반환되는 결과다.
type FormResponse struct {
	Values    map[string]string `json:"values"`
	Cancelled bool              `json:"cancelled"`
}
