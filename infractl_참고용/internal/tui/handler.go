// Package tui
// File: handler.go
// Description: agent.EventHandler를 TUI 메시지로 변환하는 핸들러 구현
// Responsibility: 에이전트 루프 이벤트를 tea.Program.Send()를 통해 TUI로 전달

package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yourorg/infractl/internal/subagent"
)

// TUIHandler는 agent.EventHandler를 구현하여 TUI에 이벤트를 전달한다.
type TUIHandler struct {
	program *tea.Program
}

// NewTUIHandler는 새 TUIHandler를 생성한다. SetProgram 호출 전까지는 no-op이다.
func NewTUIHandler() *TUIHandler {
	return &TUIHandler{}
}

// SetProgram은 이벤트 전달에 사용할 bubbletea 프로그램을 주입한다.
// tea.NewProgram() 호출 이후에 호출되어야 한다.
func (h *TUIHandler) SetProgram(p *tea.Program) {
	h.program = p
}

func (h *TUIHandler) send(msg tea.Msg) {
	if h.program != nil {
		h.program.Send(msg)
	}
}

// OnThinking은 LLM이 응답 생성을 시작했을 때 호출된다.
func (h *TUIHandler) OnThinking(tier string, model string) {
	h.send(ThinkingStartMsg{Tier: tier, Model: model})
}

// OnThinkingToken은 LLM 내부 추론 토큰을 받았을 때 호출된다.
func (h *TUIHandler) OnThinkingToken(token string) {
	h.send(ThinkingTokenMsg(token))
}

// OnToken은 LLM 스트리밍 응답 토큰을 받았을 때 호출된다.
func (h *TUIHandler) OnToken(token string) {
	h.send(TokenMsg(token))
}

// OnToolOutput은 도구 실행 중 stdout 라인을 수신했을 때 호출된다.
// ShellOutputMsg를 전송하여 app.go의 activeTools 슬라이딩 버퍼에 저장된다.
// View()에서 12줄 고정 박스로 표시되며, 도구 완료 시 박스가 사라진다.
func (h *TUIHandler) OnToolOutput(toolID string, line string) {
	h.send(ShellOutputMsg{ToolID: toolID, Line: line})
}

// OnToolStart는 도구 실행이 시작될 때 호출된다.
func (h *TUIHandler) OnToolStart(toolID string, toolName string, target string, args map[string]any) {
	h.send(ToolStartMsg{ToolID: toolID, Name: toolName, Target: target, Args: cloneToolArgs(args)})
}

// OnToolEnd는 도구 실행이 완료되었을 때 호출된다.
func (h *TUIHandler) OnToolEnd(toolID string, toolName string, result string, duration time.Duration, success bool, metadataJSON string) {
	h.send(ToolEndMsg{
		ToolID:       toolID,
		Name:         toolName,
		Result:       result,
		Duration:     duration,
		Success:      success,
		MetadataJSON: metadataJSON,
	})
}

// OnToolCancelled는 사용자가 도구 실행을 취소했을 때 호출된다.
func (h *TUIHandler) OnToolCancelled(toolID string, toolName string, reason string, duration time.Duration) {
	h.send(ToolEndMsg{
		ToolID:       toolID,
		Name:         toolName,
		Result:       reason,
		Duration:     duration,
		Success:      false,
		MetadataJSON: "",
	})
}

// OnResponse는 LLM의 최종 텍스트 응답이 완성되었을 때 호출된다.
func (h *TUIHandler) OnResponse(content string) {
	h.send(ResponseDoneMsg(content))
}

func cloneToolArgs(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// OnError는 에이전트 루프에서 에러가 발생했을 때 호출된다.
func (h *TUIHandler) OnError(err error) {
	h.send(ErrorMsg{Err: err})
}

// OnJobComplete는 백그라운드 작업이 완료되었을 때 호출된다.
func (h *TUIHandler) OnJobComplete(jobID int, description string, success bool) {
	h.send(JobCompleteMsg{JobID: jobID, Description: description, Success: success})
}

// OnTaskProposed is called when propose_task creates a PendingProposal.
func (h *TUIHandler) OnTaskProposed(title, kind, server, account string) {
	h.send(TaskProposedMsg{Title: title, Kind: kind, Server: server, Account: account})
}

// OnTaskDeclared is called when a TaskContext is declared.
func (h *TUIHandler) OnTaskDeclared(taskID, title, kind string) {
	h.send(TaskDeclaredMsg{TaskID: taskID, Title: title, Kind: kind})
}

// OnTaskStepAdvanced is called when the LLM advances to a new plan step.
func (h *TUIHandler) OnTaskStepAdvanced(taskID string, stepIndex, total int, description string) {
	h.send(TaskStepAdvancedMsg{TaskID: taskID, StepIndex: stepIndex, Total: total, Description: description})
}

// OnTaskEnded is called when the task ends (completed/failed/aborted).
func (h *TUIHandler) OnTaskEnded(taskID, status, summary string) {
	h.send(TaskEndedMsg{TaskID: taskID, Status: status, Summary: summary})
}

// OnElevationChanged is called when the current user changes in a session.
func (h *TUIHandler) OnElevationChanged(host, sessionID, currentUser string) {
	h.send(ElevationChangedMsg{Host: host, SessionID: sessionID, CurrentUser: currentUser})
}

// OnGuardViolation is called when TaskGuard blocks or warns about a tool call.
func (h *TUIHandler) OnGuardViolation(toolName, reason string, blocked bool) {
	h.send(GuardViolationMsg{ToolName: toolName, Reason: reason, Blocked: blocked})
}

// OnRAGContext는 내부 지식이 프롬프트에 주입될 때 호출된다.
func (h *TUIHandler) OnRAGContext(count int) {
	h.send(RAGContextMsg{Count: count})
}

// OnUsageUpdate는 LLM 호출 후 토큰/비용 정보를 전달한다.
func (h *TUIHandler) OnUsageUpdate(inputTokens, outputTokens int, costUSD float64, durationMs int64) {
	h.send(UsageUpdateMsg{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		CostUSD:      costUSD,
		DurationMs:   durationMs,
	})
}

// Confirm은 사용자에게 예/아니오 확인을 요청한다.
func (h *TUIHandler) Confirm(ctx context.Context, title, message string) (bool, error) {
	if h.program == nil {
		return true, nil
	}
	replyCh := make(chan SelectResult, 1)
	h.program.Send(SelectRequestMsg{
		Question: message,
		Header:   title,
		Options: []SelectOption{
			{Label: "Yes", Description: "Proceed with the operation"},
			{Label: "No", Description: "Cancel the operation"},
		},
		ReplyCh: replyCh,
	})

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case res := <-replyCh:
		// Index 0 이 "Yes" 이다.
		return res.Index == 0, nil
	}
}

// SubagentEventCallback은 서브에이전트 이벤트를 TUI로 스트리밍하는 콜백을 반환한다.
// AnalyzeTool.EventCb에 주입하여 사용한다.
func (h *TUIHandler) SubagentEventCallback() subagent.EventCallback {
	return func(e subagent.Event) {
		h.send(SubagentEventMsg{Event: e})
	}
}

