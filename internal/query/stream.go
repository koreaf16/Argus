package query

import (
	"github.com/koreaf16/argus/internal/services/llm"
	tool "github.com/koreaf16/argus/internal/tools"
)

type UIEventKind string

const (
	UIEventAssistantDelta     UIEventKind = "assistant_delta"
	UIEventThinkingDelta      UIEventKind = "thinking_delta"
	UIEventThinkingDone       UIEventKind = "thinking_done"
	UIEventToolUse            UIEventKind = "tool_use"
	UIEventToolResult         UIEventKind = "tool_result"
	UIEventPlanExecutionReady UIEventKind = "plan_execution_ready"
	UIEventNotice             UIEventKind = "notice"
	UIEventError              UIEventKind = "error"
	UIEventDone               UIEventKind = "done"
	UIEventToolDelta          UIEventKind = "tool_delta"
	UIEventPasswordPrompt     UIEventKind = "password_prompt"
	UIEventAskUserPrompt      UIEventKind = "ask_user_prompt"
	UIEventAskUserBatchPrompt UIEventKind = "ask_user_batch_prompt"
)

type PlannedStep struct {
	Tool   string
	Prompt string
}

type UIEvent struct {
	Kind UIEventKind

	Delta                string
	ToolUse              *llm.ToolUseStart
	ToolName             string
	TaskID               string
	ToolResult           string
	Prompt               string
	PlanSteps            []PlannedStep
	Notice               string
	Err                  error
	StopReason           llm.StopReason
	PasswordResponse     chan string
	Question             *tool.AskUserQuestion
	AskUserResponse      chan tool.AskUserResponse
	Questions            []tool.AskUserQuestion
	AskUserBatchResponse chan tool.AskUserBatchResponse
	InputResponse        chan string
}
