// Package tui
// File: println_render.go
// Description: Program.Println()용 채팅 이벤트 렌더링 함수 모음
// Responsibility: 사용자 입력, 도구 실행, 응답, 에러를 스크롤백 출력용 문자열로 변환

package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/subagent"
)

// renderUserInputLine은 사용자 입력을 스크롤백 출력용으로 렌더링한다.
// 형식: ❯ You\n  입력 내용 (배경색이 터미널 전체 너비까지 채워진다)
func renderUserInputLine(display string, width int) string {
	header := StyleInputPrompt.Render("❯ ") + lipgloss.NewStyle().Bold(true).Render("You")
	style := StyleUserMsg
	if width > 0 {
		style = style.Width(width)
	}
	body := style.Render("  " + display)
	return header + "\n" + body
}

// renderToolHeaderLine은 도구 시작을 스크롤백 출력용으로 렌더링한다.
// 형식: ● toolName(arg)
func renderToolHeaderLine(name, target string, args map[string]any) string {
	_ = target
	bullet := StyleClaude().Render("●")
	toolName := StyleCmdBoxToolName.Render(toolDisplayName(name))
	arg := toolDisplayArg(name, args)
	if arg != "" {
		return bullet + " " + toolName + StyleCmdBoxDim.Render("(") + arg + StyleCmdBoxDim.Render(")")
	}
	return bullet + " " + toolName
}

// renderToolSummaryLine은 도구 완료를 스크롤백 출력용으로 렌더링한다.
// 형식: ⎿ Wrote N lines to path.go
func renderToolSummaryLine(name string, args map[string]any, result string, metadataJSON string, duration time.Duration, success bool) string {
	return toolSummary(name, args, result, metadataJSON, duration, success)
}

// renderResponseText는 최종 응답을 스크롤백 출력용으로 렌더링한다.
// 각 줄 앞에 2칸 들여쓰기를 적용한다.
func renderResponseText(content string, md *mdRenderer) string {
	content = llm.SanitizeAssistantContent(content)
	if content == "" {
		return ""
	}
	lines := md.Render(content)
	if len(lines) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, line := range lines {
		sb.WriteString("   " + line)
		if i < len(lines)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// renderStreamingPreview는 스트리밍 중 미리보기를 렌더링한다.
// View()의 busy 상태에서 마지막 N줄을 표시할 때 사용한다.
// 각 줄 앞에 2칸 들여쓰기를 적용한다.
func renderStreamingPreview(content string, md *mdRenderer, cache *stableCache, maxLines int) []string {
	content = llm.SanitizeAssistantContent(content)
	if content == "" {
		return nil
	}
	lines := md.RenderStreaming(content, cache)
	if len(lines) == 0 {
		return nil
	}
	indented := make([]string, len(lines))
	for i, l := range lines {
		indented[i] = "   " + l
	}
	// 마지막 maxLines 줄만 반환
	if maxLines > 0 && len(indented) > maxLines {
		indented = indented[len(indented)-maxLines:]
	}
	return indented
}

// renderErrorLine은 에러를 스크롤백 출력용으로 렌더링한다.
func renderErrorLine(err error) string {
	return StyleError.Render(fmt.Sprintf("  Error: %s", err.Error()))
}

// renderSystemLine은 시스템 메시지를 스크롤백 출력용으로 렌더링한다.
func renderSystemLine(msg string) string {
	return StyleInfoBarDim.Render(msg)
}

// renderTurnSeparator는 채팅 턴 사이의 전체 너비 회색 구분선을 렌더링한다.
func renderTurnSeparator(width int) string {
	if width < 2 {
		return ""
	}
	return StyleInfoBarDim.Render(strings.Repeat("─", width))
}

// renderSubagentEvent는 서브에이전트 진행 이벤트를 스크롤백용 문자열로 렌더링한다.
// 형식:
//
//	EventAgentStart → "  ⎿  ● DB 에이전트 시작"
//	EventToolCall   → "     ⎿  toolName(arg)"
//	EventAgentDone  → "  ⎿  ✓ DB — Done (3.2s · 3 tool uses · 2.1k tokens)"
func renderSubagentEvent(e subagent.Event) string {
	prefix1 := StyleResponseBracket.Render("  ⎿  ")
	prefix2 := "     " + StyleResponseBracket.Render("⎿  ")
	label := strings.ToUpper(string(e.AgentType))
	if e.Server != "" && e.Server != "localhost" {
		label += "@" + e.Server
	}

	switch e.Type {
	case subagent.EventAgentStart:
		return prefix1 + StyleClaude().Render("●") + " " +
			StyleCmdBoxToolName.Render(label) + StyleCmdBoxDim.Render(" 에이전트 시작")

	case subagent.EventToolCall:
		arg := e.ToolArg
		if len(arg) > 60 {
			arg = arg[:57] + "..."
		}
		toolName := StyleCmdBoxToolName.Render(toolDisplayName(e.ToolName))
		if arg != "" {
			return prefix2 + toolName + StyleCmdBoxDim.Render("("+arg+")")
		}
		return prefix2 + toolName

	case subagent.EventAgentDone:
		dur := formatElapsedShort(e.Duration)
		var parts []string
		if e.ToolUses > 0 {
			parts = append(parts, fmt.Sprintf("%d tool uses", e.ToolUses))
		}
		total := e.InputTokens + e.OutputTokens
		if total > 0 {
			parts = append(parts, formatTokenCount(total))
		}
		parts = append(parts, dur)
		summary := strings.Join(parts, " · ")
		if e.Success {
			return prefix1 + StyleCmdBoxToolName.Render(label) + StyleCmdBoxDim.Render(" — Done ("+summary+")")
		}
		return prefix1 + StyleError.Render("✗") + " " +
			StyleCmdBoxToolName.Render(label) + StyleCmdBoxDim.Render(" — Failed ("+summary+")")
	}
	return ""
}

func renderTaskProposed(title, kind, server, account string) string {
	return StyleGeminiHint.Render(fmt.Sprintf("  [TASK PROPOSED] %s (%s) on %s@%s", title, kind, account, server))
}

func renderTaskDeclared(taskID, title, kind string) string {
	return StyleGeminiHeader.Render(fmt.Sprintf("  [TASK] %s (%s) ID: %s", title, kind, taskID))
}

func renderTaskStepAdvanced(taskID string, stepIndex, total int, description string) string {
	return StyleGeminiHint.Render(fmt.Sprintf("  Step %d/%d: %s", stepIndex, total, description))
}

func renderTaskEnded(taskID, status, summary string) string {
	icon := "✓"
	if status != "completed" {
		icon = "✗"
	}
	return StyleGeminiHeader.Render(fmt.Sprintf("  [%s] Task %s: %s", icon, status, summary))
}

func renderElevationChanged(host, sessionID, currentUser string) string {
	return StyleGeminiSubDesc.Render(fmt.Sprintf("  [ELEVATION] %s: user now %s", host, currentUser))
}

func renderGuardViolation(toolName, reason string, blocked bool) string {
	status := "WARNING"
	if blocked {
		status = "BLOCKED"
	}
	return StyleError.Render(fmt.Sprintf("  [%s] %s: %s", status, toolName, reason))
}

func renderRAGContext(count int) string {
	return StyleInfoBarDim.Render(fmt.Sprintf("  [RAG] Injected %d context items", count))
}
