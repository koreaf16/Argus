package presentation

import (
	"fmt"
	"strings"
)

func DebugLines(evt Event) []string {
	lines := make([]string, 0, 4)
	switch evt.Kind {
	case EventState:
		lines = append(lines, CanonicalStateLine(evt.Footer))
	case EventUser:
		lines = append(lines, "event: user")
		lines = append(lines, "text: "+strings.TrimSpace(evt.Text))
	case EventAssistantText:
		lines = append(lines, "event: assistant_text")
		lines = append(lines, "text: "+strings.TrimSpace(evt.Text))
	case EventAssistantDelta:
		lines = append(lines, "event: assistant_delta")
		lines = append(lines, "text: "+evt.Text)
	case EventThinkingDelta:
		lines = append(lines, "event: thinking_delta")
		lines = append(lines, "text: "+evt.Text)
	case EventThinkingDone:
		lines = append(lines, "event: thinking_done")
	case EventAssistantDone:
		lines = append(lines, "event: assistant_done")
	case EventToolUse:
		lines = append(lines, "event: tool_use")
		lines = append(lines, "tool: "+evt.ToolName)
		if strings.TrimSpace(evt.Input) != "" {
			lines = append(lines, "input: "+evt.Input)
		}
	case EventToolResult:
		lines = append(lines, "event: tool_result")
		if strings.TrimSpace(evt.ToolName) != "" {
			lines = append(lines, "tool: "+evt.ToolName)
		}
		lines = append(lines, "text: "+strings.TrimSpace(evt.Text))
	case EventNotice:
		lines = append(lines, "event: notice")
		lines = append(lines, "text: "+strings.TrimSpace(evt.Text))
	case EventSystem:
		lines = append(lines, "event: system")
		lines = append(lines, "text: "+strings.TrimSpace(evt.Text))
	case EventError:
		lines = append(lines, "event: error")
		lines = append(lines, "text: "+strings.TrimSpace(evt.Text))
	case EventApprovalRequest:
		lines = append(lines, "event: approval_request")
		lines = append(lines, "tool: "+evt.ToolName)
		if strings.TrimSpace(evt.Input) != "" {
			lines = append(lines, "input: "+evt.Input)
		}
	case EventApprovalDecision:
		lines = append(lines, "event: approval_decision")
		lines = append(lines, "decision: "+evt.Decision)
	case EventPasswordRequest:
		lines = append(lines, "event: password_request")
		lines = append(lines, "tool: "+evt.ToolName)
		if strings.TrimSpace(evt.Prompt) != "" {
			lines = append(lines, "prompt: "+strings.TrimSpace(evt.Prompt))
		}
	case EventAskUserRequest:
		lines = append(lines, "event: ask_user_request")
		lines = append(lines, "tool: "+evt.ToolName)
		if strings.TrimSpace(evt.Prompt) != "" {
			lines = append(lines, "question: "+strings.TrimSpace(evt.Prompt))
		}
		if strings.TrimSpace(evt.Input) != "" {
			lines = append(lines, "options:")
			lines = append(lines, strings.TrimSpace(evt.Input))
		}
	case EventPlanReady:
		lines = append(lines, "event: plan_ready")
		lines = append(lines, fmt.Sprintf("count: %d", evt.Count))
	case EventPlanStep:
		lines = append(lines, "event: plan_step")
		lines = append(lines, fmt.Sprintf("step: %d/%d", evt.StepIndex, evt.StepTotal))
		lines = append(lines, "tool: "+evt.ToolName)
		lines = append(lines, "prompt: "+evt.Text)
	case EventPlanDecision:
		lines = append(lines, "event: plan_decision")
		lines = append(lines, fmt.Sprintf("step: %d/%d", evt.StepIndex, evt.StepTotal))
		lines = append(lines, "decision: "+evt.Decision)
	case EventPlanResult:
		lines = append(lines, "event: plan_result")
		lines = append(lines, fmt.Sprintf("step: %d/%d", evt.StepIndex, evt.StepTotal))
		lines = append(lines, "text: "+strings.TrimSpace(evt.Text))
	case EventViewCleared:
		lines = append(lines, "event: clear_view")
	}
	return lines
}

func HumanLines(evt Event) []string {
	switch evt.Kind {
	case EventState:
		plan := "off"
		if evt.Footer.PlanMode {
			plan = "on"
		}
		line1 := fmt.Sprintf(
			"[%s] [plan %s] [%s] [todo %d] [mcp %d] [skills %d] [ws %s]",
			emptyDefault(evt.Footer.PermissionMode, "default"),
			plan,
			emptyDefault(evt.Footer.Model, "unconfigured"),
			evt.Footer.TodoCount,
			evt.Footer.MCPCount,
			evt.Footer.SkillCount,
			emptyDefault(evt.Footer.Workspace, "local"),
		)
		line2 := "Enter send  /clear keeps history  /help"
		return []string{line1, line2}
	case EventUser:
		return []string{"user", strings.TrimSpace(evt.Text), ""}
	case EventAssistantText:
		return []string{"assistant", strings.TrimSpace(evt.Text), ""}
	case EventThinkingDelta:
		return []string{"thinking", strings.TrimSpace(evt.Text), ""}
	case EventThinkingDone:
		return nil
	case EventToolUse:
		lines := []string{fmt.Sprintf("tool use  %s", evt.ToolName)}
		if strings.TrimSpace(evt.Input) != "" {
			lines = append(lines, evt.Input)
		}
		lines = append(lines, "")
		return lines
	case EventToolResult:
		text := strings.TrimSpace(evt.Text)
		// distilled output: "[전체 출력: /path/to/file]" 패턴 분리
		artifactHint := ""
		if idx := strings.LastIndex(text, "\n\n[전체 출력:"); idx >= 0 {
			artifactHint = strings.TrimSpace(text[idx+2:])
			text = strings.TrimSpace(text[:idx])
		} else if idx := strings.LastIndex(text, "\n\n[Full output:"); idx >= 0 {
			artifactHint = strings.TrimSpace(text[idx+2:])
			text = strings.TrimSpace(text[:idx])
		}
		lines := []string{fmt.Sprintf("tool result  %s", emptyDefault(evt.ToolName, "-")), text}
		if artifactHint != "" {
			lines = append(lines, artifactHint)
		}
		lines = append(lines, "")
		return lines
	case EventNotice:
		return []string{"notice", strings.TrimSpace(evt.Text), ""}
	case EventSystem:
		return []string{"system", strings.TrimSpace(evt.Text), ""}
	case EventError:
		return []string{"error", strings.TrimSpace(evt.Text), ""}
	case EventApprovalRequest:
		lines := []string{"[permission required]", "tool: " + evt.ToolName}
		if strings.TrimSpace(evt.Input) != "" {
			lines = append(lines, evt.Input)
		}
		lines = append(lines, "[Allow] [Deny]", "")
		return lines
	case EventApprovalDecision:
		return []string{"[permission decision] " + evt.Decision, ""}
	case EventPasswordRequest:
		lines := []string{"[password required]", "tool: " + evt.ToolName}
		if strings.TrimSpace(evt.Prompt) != "" {
			lines = append(lines, "prompt: "+strings.TrimSpace(evt.Prompt))
		}
		lines = append(lines, "")
		return lines
	case EventAskUserRequest:
		lines := []string{"[question required]", "tool: " + evt.ToolName}
		if strings.TrimSpace(evt.Prompt) != "" {
			lines = append(lines, "question: "+strings.TrimSpace(evt.Prompt))
		}
		if strings.TrimSpace(evt.Input) != "" {
			lines = append(lines, evt.Input)
		}
		lines = append(lines, "")
		return lines
	case EventPlanReady:
		return []string{fmt.Sprintf("notice\napproved steps: %d\n", evt.Count)}
	case EventPlanStep:
		return []string{fmt.Sprintf("[step %d/%d] %s: %s", evt.StepIndex, evt.StepTotal, evt.ToolName, evt.Text)}
	case EventPlanDecision:
		return []string{fmt.Sprintf("[step %d/%d] decision: %s", evt.StepIndex, evt.StepTotal, evt.Decision)}
	case EventPlanResult:
		return []string{fmt.Sprintf("[step %d/%d] %s", evt.StepIndex, evt.StepTotal, evt.Text)}
	case EventViewCleared:
		return []string{"", "----- view cleared -----", ""}
	default:
		return nil
	}
}
