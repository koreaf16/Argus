// Package cli
// File: repl_handler.go
// Description: REPL 및 디버그 모드를 위한 에이전트 이벤트 핸들러 구현
// Responsibility: 에이전트 이벤트를 표준 출력으로 시각화하고, 질문/폼 요청에 대해 표준 입력으로 응답 처리

package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/yourorg/infractl/internal/agent"
	"github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/privilege"
	"github.com/yourorg/infractl/internal/tools"
)

var (
	styleThinking = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	styleTool     = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	styleUI       = lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true)
	styleError    = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	styleSuccess  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	stylePrompt   = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
)

// REPLHandler는 에이전트의 이벤트를 콘솔에 출력하고 입력을 처리한다.
type REPLHandler struct {
	IsDebug               bool
	IsJSON                bool
	ShowAssistantResponse bool
	scanner               *bufio.Scanner
}

var _ agent.EventHandler = (*REPLHandler)(nil)
var _ agent.QuestionHandler = (*REPLHandler)(nil)
var _ privilege.PromptHandler = (*REPLHandler)(nil)
var _ connector.ConfirmationHandler = (*REPLHandler)(nil)

func NewREPLHandler(debug, json bool) *REPLHandler {
	return &REPLHandler{
		IsDebug:               debug,
		IsJSON:                json,
		ShowAssistantResponse: true,
		scanner:               bufio.NewScanner(os.Stdin),
	}
}

func (h *REPLHandler) logJSON(event string, data any) {
	if !h.IsJSON {
		return
	}
	msg := map[string]any{
		"event": event,
		"data":  data,
		"time":  time.Now().Format(time.RFC3339),
	}
	b, _ := json.Marshal(msg)
	_, _ = os.Stdout.Write(append(b, '\n'))
}

// Confirm implements connector.ConfirmationHandler
func (h *REPLHandler) Confirm(ctx context.Context, title, message string) (bool, error) {
	h.logJSON("ui_confirm", map[string]string{
		"title":   title,
		"message": message,
	})
	if !h.IsJSON {
		fmt.Printf("\n%s %s\n%s\n", styleUI.Render("[CONFIRM]"), title, message)
		fmt.Printf("%s Proceed? [y/N]: ", stylePrompt.Render(">"))
	}

	if h.scanner.Scan() {
		input := strings.ToLower(strings.TrimSpace(h.scanner.Text()))
		approved := input == "y" || input == "yes"
		h.logJSON("ui_confirm_result", approved)
		return approved, nil
	}
	return false, fmt.Errorf("stdin closed")
}

func (h *REPLHandler) OnThinking(tier string, model string) {
	h.logJSON("thinking_start", map[string]string{"tier": tier, "model": model})
	if !h.IsJSON {
		fmt.Printf("%sThinking (%s/%s)...\n", styleThinking.Render("[ENGINE] "), tier, model)
	}
}

func (h *REPLHandler) OnThinkingToken(token string) {
	if h.IsDebug && !h.IsJSON {
		fmt.Print(styleThinking.Render(token))
	}
}

func (h *REPLHandler) OnToken(token string) {
	if !h.IsJSON && h.ShowAssistantResponse {
		fmt.Print(token)
	}
}

func (h *REPLHandler) OnToolStart(toolID string, toolName string, target string, args map[string]any) {
	h.logJSON("tool_start", map[string]any{
		"id":     toolID,
		"name":   toolName,
		"target": target,
		"args":   args,
	})
	if !h.IsJSON {
		targetStr := target
		if targetStr == "" {
			targetStr = "localhost"
		}
		fmt.Printf("\n%sExecuting %s on %s...\n", styleTool.Render("[TOOL] "), toolName, targetStr)
		if h.IsDebug {
			argB, _ := json.MarshalIndent(args, "  ", "  ")
			fmt.Printf("  Args: %s\n", string(argB))
		}
	}
}

func (h *REPLHandler) OnToolOutput(toolID string, line string) {
	if !h.IsJSON {
		fmt.Printf("  %s %s\n", styleTool.Render("|"), line)
	}
}

func (h *REPLHandler) OnToolEnd(toolID string, toolName string, result string, duration time.Duration, success bool, metadataJSON string) {
	h.logJSON("tool_end", map[string]any{
		"id":       toolID,
		"name":     toolName,
		"success":  success,
		"duration": duration.String(),
		"result":   result,
	})
	if !h.IsJSON {
		status := styleSuccess.Render("Success")
		if !success {
			status = styleError.Render("Failed")
		}
		fmt.Printf("%s%s completed (%s) [%s]\n", styleTool.Render("[TOOL] "), toolName, duration.Round(time.Millisecond), status)
		if h.IsDebug && !success {
			fmt.Printf("  Error: %s\n", result)
		}
	}
}

func (h *REPLHandler) OnToolCancelled(toolID string, toolName string, reason string, duration time.Duration) {
	h.logJSON("tool_cancelled", map[string]any{"id": toolID, "name": toolName, "reason": reason})
	if !h.IsJSON {
		fmt.Printf("%s%s cancelled: %s\n", styleError.Render("[TOOL] "), toolName, reason)
	}
}

func (h *REPLHandler) OnResponse(content string) {
	h.logJSON("response", content)
	if !h.IsJSON && h.ShowAssistantResponse {
		fmt.Printf("\n%s\n%s\n", stylePrompt.Render("[ASSISTANT]"), content)
	}
}

func (h *REPLHandler) OnError(err error) {
	h.logJSON("error", err.Error())
	if !h.IsJSON {
		fmt.Printf("\n%s %v\n", styleError.Render("[ERROR]"), err)
	}
}

func (h *REPLHandler) OnUsageUpdate(inputTokens, outputTokens int, costUSD float64, durationMs int64) {
	h.logJSON("usage", map[string]any{
		"input":    inputTokens,
		"output":   outputTokens,
		"cost":     costUSD,
		"duration": durationMs,
	})
	if h.IsDebug && !h.IsJSON {
		fmt.Printf("%sTokens: %d in / %d out | Cost: $%.4f\n", styleThinking.Render("[USAGE] "), inputTokens, outputTokens, costUSD)
	}
}

func (h *REPLHandler) OnJobComplete(jobID int, description string, success bool) {
	h.logJSON("job_complete", map[string]any{"id": jobID, "desc": description, "success": success})
	if !h.IsJSON {
		fmt.Printf("%sJob #%d (%s) completed: %v\n", styleUI.Render("[JOB] "), jobID, description, success)
	}
}

func (h *REPLHandler) OnRAGContext(count int) {
	h.logJSON("rag_context", count)
	if h.IsDebug && !h.IsJSON {
		fmt.Printf("%sInjected %d context items from RAG\n", styleThinking.Render("[RAG] "), count)
	}
}

// RequestQuestion implements agent.QuestionHandler
func (h *REPLHandler) RequestQuestion(ctx context.Context, req tools.QuestionRequest) (tools.QuestionResponse, error) {
	h.logJSON("ui_question", req)
	if !h.IsJSON {
		fmt.Printf("\n%s %s\n", styleUI.Render("[QUESTION]"), req.Question)
		for i, opt := range req.Options {
			fmt.Printf("  %d) %s (%s)\n", i+1, opt.Label, opt.Description)
		}
		fmt.Printf("%s Enter choice (1-%d) or response: ", stylePrompt.Render(">"), len(req.Options))
	}

	if h.scanner.Scan() {
		input := strings.TrimSpace(h.scanner.Text())
		var idx int
		if _, err := fmt.Sscanf(input, "%d", &idx); err == nil && idx > 0 && idx <= len(req.Options) {
			return tools.QuestionResponse{
				SelectedIndex: idx - 1,
				SelectedLabel: req.Options[idx-1].Label,
			}, nil
		}
		return tools.QuestionResponse{IsOther: true, UserInput: input}, nil
	}
	return tools.QuestionResponse{}, fmt.Errorf("stdin closed")
}

// RequestPassword implements privilege.PromptHandler
func (h *REPLHandler) RequestPassword(ctx context.Context, req privilege.PromptRequest) (privilege.PromptResponse, error) {
	h.logJSON("ui_question", map[string]any{
		"header":   "Privilege Password Required",
		"question": fmt.Sprintf("Enter password for %s@%s (%s):", req.User, req.Target, req.Method),
	})
	if !h.IsJSON {
		fmt.Printf("\n%s Password for %s@%s (%s): ", styleUI.Render("[AUTH]"), req.User, req.Target, req.Method)
	}

	if h.scanner.Scan() {
		return privilege.PromptResponse{Password: strings.TrimSpace(h.scanner.Text())}, nil
	}
	return privilege.PromptResponse{Abort: true}, fmt.Errorf("stdin closed")
}

func (h *REPLHandler) RequestForm(ctx context.Context, req tools.FormRequest) (tools.FormResponse, error) {
	h.logJSON("ui_form", req)
	results := make(map[string]string)
	for _, field := range req.Fields {
		if !h.IsJSON {
			fmt.Printf("  %s (%s): ", field.Label, field.Placeholder)
		}
		if h.scanner.Scan() {
			results[field.Name] = strings.TrimSpace(h.scanner.Text())
		} else {
			return tools.FormResponse{}, fmt.Errorf("stdin closed")
		}
	}
	return tools.FormResponse{Values: results}, nil
}

func (h *REPLHandler) RequestIdleInput(ctx context.Context, req agent.IdleInputRequest) (agent.IdleInputResponse, error) {
	h.logJSON("ui_idle_input", req)
	if h.scanner.Scan() {
		input := strings.TrimSpace(h.scanner.Text())
		if strings.ToUpper(input) == "ABORT" {
			return agent.IdleInputResponse{Abort: true}, nil
		}
		return agent.IdleInputResponse{Input: input}, nil
	}
	return agent.IdleInputResponse{Abort: true}, fmt.Errorf("stdin closed")
}

func (h *REPLHandler) OnTaskProposed(title, kind, server, account string)                  {}
func (h *REPLHandler) OnTaskDeclared(taskID, title, kind string)                           {}
func (h *REPLHandler) OnTaskStepAdvanced(taskID string, stepIndex, total int, desc string) {}
func (h *REPLHandler) OnTaskEnded(taskID, status, summary string)                          {}
func (h *REPLHandler) OnElevationChanged(host, sessionID, currentUser string)              {}
func (h *REPLHandler) OnGuardViolation(toolName, reason string, blocked bool)              {}
