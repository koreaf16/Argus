package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/yourorg/infractl/internal/privilege"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/tools"
)

type RichHandler struct {
	renderer *DirectRenderer
	state    *SessionState
	done     chan struct{}
}

func NewRichHandler(state *SessionState) *RichHandler {
	return &RichHandler{state: state}
}

func (h *RichHandler) StartTurn(_ context.Context) chan struct{} {
	h.renderer = NewDirectRenderer(h.state)
	h.done = make(chan struct{})
	return h.done
}

func (h *RichHandler) FinishTurn() {
	if h.renderer != nil {
		h.renderer.Finish()
		h.renderer = nil
	}
	if h.done != nil {
		close(h.done)
		h.done = nil
	}
}

func (h *RichHandler) OnThinking(tier string, model string) {
	if h.renderer != nil {
		h.renderer.OnThinking(tier, model)
	}
}

func (h *RichHandler) OnThinkingToken(token string) {
	if h.renderer != nil {
		h.renderer.OnThinkingToken(token)
	}
}

func (h *RichHandler) OnToken(token string) {
	if h.renderer != nil {
		h.renderer.OnToken(token)
	}
}

func (h *RichHandler) OnToolOutput(toolID, line string) {
	if h.renderer != nil {
		h.renderer.OnToolOutput(toolID, line)
	}
}

func (h *RichHandler) OnToolStart(toolID, toolName, target string, args map[string]any) {
	if h.renderer != nil {
		h.renderer.OnToolStart(toolID, toolName, target, args)
	}
}

func (h *RichHandler) OnToolEnd(toolID, toolName, result string, duration time.Duration, success bool, metadataJSON string) {
	if h.renderer != nil {
		h.renderer.OnToolEnd(toolID, toolName, result, duration, success, metadataJSON)
	}
}

func (h *RichHandler) OnToolCancelled(toolID, toolName, reason string, duration time.Duration) {
	if h.renderer != nil {
		h.renderer.OnToolCancelled(toolID, toolName, reason, duration)
	}
}

func (h *RichHandler) OnResponse(content string) {
	if h.renderer != nil {
		h.renderer.OnResponse(content)
	}
}

func (h *RichHandler) OnError(err error) {
	if h.renderer != nil {
		h.renderer.OnError(err)
	}
}

func (h *RichHandler) OnUsageUpdate(inputTokens, outputTokens int, costUSD float64, _ int64) {
	if h.renderer != nil {
		h.renderer.OnUsageUpdate(inputTokens, outputTokens, costUSD)
	}
}

func (h *RichHandler) OnRAGContext(_ int) {}

func (h *RichHandler) OnJobComplete(jobID int, description string, success bool) {
	if h.renderer == nil {
		return
	}
	bracket := StyleResponseBracket.Render("  > ")
	var text string
	if success {
		text = fmt.Sprintf("%sbackground job #%d complete: %s", bracket, jobID, description)
	} else {
		text = fmt.Sprintf("%s%s background job #%d complete: %s", bracket, StyleError.Render("x"), jobID, description)
	}
	h.renderer.PrintNotification(text)
}

func (h *RichHandler) OnActiveServerChange(srv *store.Server) {
	if h.renderer == nil {
		return
	}
	bracket := StyleResponseBracket.Render("  > ")
	var text string
	if srv == nil {
		text = bracket + StyleCmdBoxDim.Render("[active workspace] cleared")
	} else {
		text = bracket + StyleCmdBoxDim.Render(fmt.Sprintf("active workspace: %s (%s:%d, dir: %s)", srv.Name, srv.Host, srv.Port, srv.WorkspaceDir))
	}
	h.renderer.PrintNotification(text)
}

type RichPrivilegeHandler struct {
	handler *RichHandler
	inner   privilege.PromptHandler
}

func (h *RichHandler) NewPrivilegePromptHandler(inner privilege.PromptHandler) privilege.PromptHandler {
	return &RichPrivilegeHandler{handler: h, inner: inner}
}

func (p *RichPrivilegeHandler) RequestPassword(ctx context.Context, req privilege.PromptRequest) (privilege.PromptResponse, error) {
	if p.handler.renderer != nil {
		p.handler.renderer.PauseForPrompt()
		defer func() {
			label := p.handler.renderer.thinkingLabel
			if label == "" {
				label = "thinking..."
			}
			p.handler.renderer.ResumeAfterPrompt(label)
		}()
	}
	return p.inner.RequestPassword(ctx, req)
}

// OnTaskProposed prints a notification that a task proposal is waiting.
func (h *RichHandler) OnTaskProposed(title, kind, server, account string) {
	if h.renderer == nil {
		return
	}
	bracket := StyleResponseBracket.Render("  > ")
	text := bracket + StyleTaskDim.Render(fmt.Sprintf(
		"[작업 제안] %s (%s) — %s/%s — 'yes'로 확인, '취소'로 거부",
		title, kind, server, account,
	))
	h.renderer.PrintNotification(text)
}

// OnTaskDeclared prints a notification that a task has been confirmed.
func (h *RichHandler) OnTaskDeclared(taskID, title, kind string) {
	if h.renderer == nil {
		return
	}
	bracket := StyleResponseBracket.Render("  > ")
	h.renderer.PrintNotification(bracket + StyleSuccess.Render(
		fmt.Sprintf("[작업 확정] %s (%s) ID:%s", title, kind, taskID),
	))
}

// OnTaskStepAdvanced prints a notification that a task step has advanced.
func (h *RichHandler) OnTaskStepAdvanced(taskID string, stepIndex, total int, description string) {
	if h.renderer == nil {
		return
	}
	bracket := StyleResponseBracket.Render("  > ")
	h.renderer.PrintNotification(bracket + StyleTaskDim.Render(
		fmt.Sprintf("[단계 %d/%d] %s", stepIndex+1, total, description),
	))
}

// OnTaskEnded prints a notification that a task has ended.
func (h *RichHandler) OnTaskEnded(taskID, status, summary string) {
	if h.renderer == nil {
		return
	}
	bracket := StyleResponseBracket.Render("  > ")
	style := StyleSuccess
	if status == "failed" || status == "aborted" {
		style = StyleError
	}
	text := fmt.Sprintf("[작업 종료] %s — %s", status, taskID)
	if summary != "" {
		text += ": " + summary
	}
	h.renderer.PrintNotification(bracket + style.Render(text))
}

// OnElevationChanged prints a notification that privilege elevation has changed.
func (h *RichHandler) OnElevationChanged(host, sessionID, currentUser string) {
	if h.renderer == nil {
		return
	}
	bracket := StyleResponseBracket.Render("  > ")
	h.renderer.PrintNotification(bracket + StyleWarning.Render(
		fmt.Sprintf("[권한 변경] %s@%s (세션: %s)", currentUser, host, sessionID),
	))
}

// OnGuardViolation prints a notification that a task guard rule was triggered.
func (h *RichHandler) OnGuardViolation(toolName, reason string, blocked bool) {
	if h.renderer == nil {
		return
	}
	h.renderer.OnGuardViolation(toolName, reason, blocked)
}

func (h *RichHandler) RequestQuestion(ctx context.Context, req tools.QuestionRequest) (tools.QuestionResponse, error) {
	if h.renderer != nil {
		h.renderer.PauseForPrompt()
		defer func() {
			label := h.renderer.thinkingLabel
			if label == "" {
				label = "thinking..."
			}
			h.renderer.ResumeAfterPrompt(label)
		}()
	}

	allowCustomInput := req.AllowCustomInput || len(req.Options) == 0
	opts := make([]SelectOption, len(req.Options))
	for i, opt := range req.Options {
		opts[i] = SelectOption{
			Label:       opt.Label,
			Description: opt.Description,
			HideOther:   !allowCustomInput,
		}
	}

	width := 80
	if h.state != nil && h.state.Width > 0 {
		width = h.state.Width
	}

	doneCh := make(chan SelectResult, 1)
	go func() {
		doneCh <- RunSelectWithHeader(req.Question, opts, width, req.Header)
	}()

	select {
	case <-ctx.Done():
		fmt.Println()
		return tools.QuestionResponse{SelectedIndex: -1}, nil
	case result := <-doneCh:
		if result.Index < 0 {
			if result.IsOther {
				return tools.QuestionResponse{
					IsOther:   true,
					UserInput: result.Label,
				}, nil
			}
			return tools.QuestionResponse{SelectedIndex: -1}, nil
		}

		return tools.QuestionResponse{
			SelectedIndex: result.Index,
			SelectedLabel: result.Label,
		}, nil
	}
}
