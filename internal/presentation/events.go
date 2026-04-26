package presentation

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"github.com/koreaf16/argus/internal/query"
	"github.com/koreaf16/argus/internal/redact"
	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/state"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/utils/permissions"
)

type EventKind string

const (
	EventState            EventKind = "state"
	EventUser             EventKind = "user"
	EventAssistantText    EventKind = "assistant_text"
	EventAssistantDelta   EventKind = "assistant_delta"
	EventAssistantDone    EventKind = "assistant_done"
	EventThinkingDelta    EventKind = "thinking_delta"
	EventThinkingDone     EventKind = "thinking_done"
	EventToolUse          EventKind = "tool_use"
	EventToolDelta        EventKind = "tool_delta"
	EventToolResult       EventKind = "tool_result"
	EventNotice           EventKind = "notice"
	EventSystem           EventKind = "system"
	EventError            EventKind = "error"
	EventApprovalRequest  EventKind = "approval_request"
	EventApprovalDecision EventKind = "approval_decision"
	EventPasswordRequest  EventKind = "password_request"
	EventAskUserRequest   EventKind = "ask_user_request"
	EventPlanReady        EventKind = "plan_ready"
	EventPlanStep         EventKind = "plan_step"
	EventPlanDecision     EventKind = "plan_decision"
	EventPlanResult       EventKind = "plan_result"
	EventViewCleared      EventKind = "clear_view"
)

type FooterState struct {
	PermissionMode     string
	PermissionSymbol   string
	PermissionTitle    string
	PlanMode           bool
	Model              string
	ProviderName       string
	EffortSuffix       string
	TodoCount          int
	MCPCount           int
	SkillCount         int
	Workspace          string
	SessionID          string
	SessionShortID     string
	CWD                string
	ContextUsedPercent int
	ContextUsedLabel   string
	MemoryUsageLabel   string
	DiffAdded          int
	DiffRemoved        int
	TokensUsed         int
}

type Event struct {
	Kind EventKind

	Text     string
	ToolName string
	TaskID   string
	Input    string
	Prompt   string

	Decision string

	Count     int
	StepIndex int
	StepTotal int

	InputResponse chan string

	AskUserBatchResponse chan tool.AskUserBatchResponse

	Footer FooterState
}

func BuildFooterState(app *state.AppState, cwd string) FooterState {
	out := FooterState{
		CWD: strings.TrimSpace(cwd),
	}
	if app == nil {
		if out.CWD == "" {
			out.CWD = "."
		}
		return out
	}
	mode := app.GetPermissionMode()
	out.PermissionMode = string(mode)
	out.PermissionSymbol = permissions.PermissionModeSymbol(mode)
	out.PermissionTitle = permissions.PermissionModeTitle(mode)
	out.PlanMode = app.InPlanMode()
	_, display, _ := app.ActiveModel()
	if strings.TrimSpace(display) == "" {
		out.Model = "unconfigured"
	} else {
		out.Model = strings.TrimSpace(display)
	}
	provider := strings.TrimSpace(app.ActiveModelProvider())
	if provider == "" {
		provider = "unknown"
	}
	out.ProviderName = provider
	effort := app.GetEffortLevel()
	out.EffortSuffix = "● " + effort + " · /effort"
	out.SessionID = strings.TrimSpace(app.SessionID())
	out.SessionShortID = shortenSessionID(out.SessionID)
	out.MCPCount = len(app.ActiveMCPServers())
	out.SkillCount = len(app.ActiveSkills())
	out.ContextUsedPercent = app.ContextUsedPercent()
	out.ContextUsedLabel = fmt.Sprintf("%d%% used", out.ContextUsedPercent)
	out.MemoryUsageLabel = formatMemoryUsageLabel()
	workspace := strings.TrimSpace(app.ActiveWorkspace())
	if workspace == "" {
		workspace = "local"
	}
	out.Workspace = workspace
	sessionID := out.SessionID
	if sessionID == "" {
		sessionID = "default"
	}
	out.TodoCount = len(app.Todos(sessionID))
	if out.CWD == "" {
		out.CWD = "."
	}
	return out
}

func FooterEqual(a, b FooterState) bool {
	return a == b
}

func ReplayEvents(messages []llm.Message) []Event {
	out := make([]Event, 0, len(messages))
	for _, msg := range messages {
		for _, block := range msg.Content {
			switch msg.Role {
			case llm.RoleUser:
				switch block.Type {
				case llm.ContentText:
					if strings.TrimSpace(block.Text) != "" {
						out = append(out, Event{Kind: EventUser, Text: block.Text})
					}
				case llm.ContentToolResult:
					out = append(out, Event{
						Kind:     EventToolResult,
						ToolName: block.Name,
						Text:     block.Text,
					})
				}
			case llm.RoleAssistant:
				switch block.Type {
				case llm.ContentText:
					if strings.TrimSpace(block.Text) != "" {
						out = append(out, Event{Kind: EventAssistantText, Text: block.Text})
					}
				case llm.ContentToolUse:
					out = append(out, Event{
						Kind:     EventToolUse,
						ToolName: block.Name,
						Input:    FormatInputJSON(block.Input),
					})
				}
			}
		}
	}
	return out
}

func FromUIEvent(evt query.UIEvent) (Event, bool) {
	switch evt.Kind {
	case query.UIEventAssistantDelta:
		return Event{Kind: EventAssistantDelta, Text: evt.Delta}, true
	case query.UIEventThinkingDelta:
		return Event{Kind: EventThinkingDelta, Text: evt.Delta}, true
	case query.UIEventThinkingDone:
		return Event{Kind: EventThinkingDone}, true
	case query.UIEventToolUse:
		if evt.ToolUse == nil {
			return Event{}, false
		}
		return Event{
			Kind:     EventToolUse,
			ToolName: evt.ToolUse.Name,
			TaskID:   strings.TrimSpace(evt.TaskID),
			Input:    FormatInputJSON(evt.ToolUse.Input),
		}, true
	case query.UIEventToolDelta:
		return Event{
			Kind:          EventToolDelta,
			ToolName:      evt.ToolName,
			TaskID:        strings.TrimSpace(evt.TaskID),
			Text:          evt.Delta,
			InputResponse: evt.InputResponse,
		}, true
	case query.UIEventToolResult:
		return Event{
			Kind:     EventToolResult,
			ToolName: evt.ToolName,
			TaskID:   strings.TrimSpace(evt.TaskID),
			Text:     evt.ToolResult,
		}, true
	case query.UIEventNotice:
		return Event{Kind: EventNotice, Text: evt.Notice}, true
	case query.UIEventError:
		if evt.Err != nil {
			return Event{Kind: EventError, Text: evt.Err.Error()}, true
		}
		return Event{Kind: EventError}, true
	case query.UIEventDone:
		return Event{Kind: EventAssistantDone}, true
	case query.UIEventPasswordPrompt:
		return Event{
			Kind:     EventPasswordRequest,
			ToolName: evt.ToolName,
			Prompt:   evt.Prompt,
		}, true
	case query.UIEventAskUserPrompt:
		if evt.Question == nil {
			return Event{}, false
		}
		// 단일 질문도 배치 응답 채널을 재사용할 수 있도록 변환 필요 (일단 패스하거나 나중에 통합)
		return Event{
			Kind:     EventAskUserRequest,
			ToolName: evt.ToolName,
			Prompt:   strings.TrimSpace(evt.Question.Question),
			Input:    formatAskUserOptions(evt.Question.Options),
		}, true
	case query.UIEventAskUserBatchPrompt:
		if len(evt.Questions) == 0 {
			return Event{}, false
		}
		prompt, input := formatAskUserBatch(evt.Questions)
		return Event{
			Kind:                 EventAskUserRequest,
			ToolName:             evt.ToolName,
			Prompt:               prompt,
			Input:                input,
			AskUserBatchResponse: evt.AskUserBatchResponse,
		}, true
	case query.UIEventPlanExecutionReady:
		return Event{Kind: EventPlanReady, Count: len(evt.PlanSteps)}, true
	default:
		return Event{}, false
	}
}

func FormatInputJSON(input json.RawMessage) string {
	return strings.TrimSpace(string(redact.RedactJSON(input)))
}

func CanonicalStateLine(s FooterState) string {
	plan := "off"
	if s.PlanMode {
		plan = "on"
	}
	return fmt.Sprintf(
		"state: permission=%s plan=%s model=%s todo=%d mcp=%d skills=%d workspace=%s session=%s cwd=%s",
		emptyDefault(s.PermissionMode, "default"),
		plan,
		emptyDefault(s.Model, "unconfigured"),
		s.TodoCount,
		s.MCPCount,
		s.SkillCount,
		emptyDefault(s.Workspace, "local"),
		emptyDefault(s.SessionID, "-"),
		emptyDefault(s.CWD, "."),
	)
}

func emptyDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return strings.TrimSpace(v)
}

func shortenSessionID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "-"
	}
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func formatMemoryUsageLabel() string {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	mb := float64(mem.Alloc) / (1024 * 1024)
	return fmt.Sprintf("%.1f MB", mb)
}

func formatAskUserOptions(options []tool.AskUserOption) string {
	if len(options) == 0 {
		return ""
	}
	lines := make([]string, 0, len(options))
	for i, opt := range options {
		label := strings.TrimSpace(opt.Label)
		value := strings.TrimSpace(opt.Value)
		desc := strings.TrimSpace(opt.Description)
		if label == "" {
			label = value
		}
		if label == "" {
			continue
		}
		line := fmt.Sprintf("%d. %s", i+1, label)
		if desc != "" {
			line += " - " + desc
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func formatAskUserBatch(questions []tool.AskUserQuestion) (prompt, input string) {
	if len(questions) == 0 {
		return "", ""
	}
	if len(questions) == 1 {
		return strings.TrimSpace(questions[0].Question), formatAskUserOptions(questions[0].Options)
	}
	parts := make([]string, 0, len(questions))
	for i, q := range questions {
		header := strings.TrimSpace(q.Header)
		if header == "" {
			header = fmt.Sprintf("Q%d", i+1)
		}
		parts = append(parts, fmt.Sprintf("%d. %s", i+1, header))
	}
	return fmt.Sprintf("%d questions", len(questions)), strings.Join(parts, "\n")
}
