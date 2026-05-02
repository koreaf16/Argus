package tui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/koreaf16/argus/internal/components/logo"
	"github.com/koreaf16/argus/internal/components/promptinput"
	"github.com/koreaf16/argus/internal/constants"
	"github.com/koreaf16/argus/internal/tui/markdown"
	"github.com/koreaf16/argus/internal/tui/toolui"
	"github.com/koreaf16/argus/internal/types"
	"github.com/muesli/reflow/wordwrap"
)

const stalledAfter = 3 * time.Second

// claude_cli figures.ts와 동일한 글리프를 사용한다.
// 도구 호출은 BLACK_CIRCLE(`●`, U+25CF — Windows/Linux 변형), 결과 branch는 `⎿`(U+23BF).
const (
	IconToolUse      = "●"
	IconResultBranch = "⎿"
	IconSuccess      = ""
	IconPending      = ""
	IconExecuting    = ""
	IconThinking     = "*"
	IconConfirming   = ""
	IconCanceled     = ""
	IconError        = ""
)

// asciiSpinnerFrames는 reduced-motion 모드에서 사용하는 ASCII 폴백 스피너.
var asciiSpinnerFrames = []string{"|", "/", "-", "\\"}

// spinnerFrames는 claude_cli SpinnerGlyph와 동일한 팰린드롬 프레임 배열이다 (Windows 변형).
// 정방향: ['·','✢','*','✶','✻','✽'], 역방향을 이어 붙여 부드러운 반복 애니메이션을 만든다.
var spinnerFrames = []string{"·", "✢", "*", "✶", "✻", "✽", "✽", "✻", "✶", "*", "✢", "·"}

func (m uiModel) getRainbowColor() string {
	if !m.signatureMotionEnabled() {
		return m.theme.StatusLeftColor
	}
	colors := m.theme.GlimmerTrailColors
	if len(colors) == 0 {
		return m.theme.StatusLeftColor
	}
	idx := m.motionFrame(len(colors), 2)
	return colors[idx]
}

func (m uiModel) motionEnabled() bool {
	if m.theme.DisableANSI {
		return false
	}
	if !m.ui.Motion.Enabled || m.ui.Motion.Level == "static" {
		return false
	}
	return true
}

func (m uiModel) signatureMotionEnabled() bool {
	return m.motionEnabled() && m.ui.Motion.Signature
}

func (m uiModel) motionFrame(modulo int, speedDiv int) int {
	if modulo <= 0 {
		return 0
	}
	if speedDiv <= 0 {
		speedDiv = 1
	}
	if !m.motionEnabled() {
		return 0
	}
	frame := m.animFrame / speedDiv
	if m.ui.Motion.Reduced || m.ui.Motion.Level == "restrained" {
		frame = m.animFrame / (speedDiv * 3)
	}
	return frame % modulo
}

func (m uiModel) animatedDots() string {
	if !m.motionEnabled() {
		return "..."
	}
	// 3단계 점 애니메이션: . -> .. -> ...
	count := (m.animFrame / 4) % 4
	if count == 0 { count = 1 }
	return strings.Repeat(".", count)
}

func (m uiModel) slidingDots() string {
	if !m.motionEnabled() {
		return "..."
	}
	// 점이 흐르는 애니메이션: .   ->  .  ->   . ->    .
	frames := []string{".  ", " . ", "  .", "   "}
	return frames[(m.animFrame/3)%len(frames)]
}

func (m uiModel) renderTranscriptEntry(e transcriptEntry) string {
	return m.renderTranscriptEntryAt(-1, e)
}

func (m uiModel) renderTranscriptEntryAt(idx int, e transcriptEntry) string {
	streamingAssistant := idx >= 0 && m.assistantOpen && idx == m.assistantStreamIdx
	streamingThinking := idx >= 0 && m.thinkingOpen && idx == m.thinkingStreamIdx
	streamingTool := idx >= 0 && m.toolUseOpen && idx == m.toolUseStreamIdx
	stalled := m.isStreamStalled()
	rendered := m.renderEntry(e, streamingAssistant, streamingThinking, streamingTool, stalled)
	if rendered == "" {
		return ""
	}

	// claude_cli AssistantToolUseMessage 패턴을 모든 주요 엔트리에 확장 적용:
	//   `● Bash(ls -la)`            (헤드라인 — `●` + 공백 + 도구명(인자))
	//   `● Plan mode on`            (시스템 메시지도 ● 아이콘 통일)
	//
	// tool_use, tool_group, system, notice, error 등은 첫 줄 앞에 `● ` 색상 prefix를 추가한다.
	switch e.Kind {
	case "user", "assistant", "tool_result", "parallel_group":
		// 자체 패딩 사용
	case "tool_use", "tool_group", "system", "notice", "error", "plan":
		// 아이콘 색상 및 애니메이션 결정
		var iconColor string
		glyph := IconToolUse

		switch {
		case e.Failed:
			iconColor = m.theme.ErrorTitleColor
			// 실패 시 1초 주기로 부드럽게 점멸 (20프레임 내외 순환)
			if m.motionEnabled() {
				blink := m.animFrame % 20
				if blink > 10 {
					iconColor = m.theme.MutedColor // 꺼진 느낌
				}
			}
		case e.IsActive:
			iconColor = m.ClaudeAccentColor()
			if m.signatureMotionEnabled() {
				iconColor = m.getRainbowColor()
			}
			// 동작 중에는 스피너 애니메이션
			if m.motionEnabled() {
				if m.ui.Motion.Reduced || m.ui.Motion.Level == "restrained" {
					glyph = asciiSpinnerFrames[m.motionFrame(len(asciiSpinnerFrames), 2)]
				} else {
					glyph = spinnerFrames[m.motionFrame(len(spinnerFrames), 1)]
				}
			}
		default:
			// 성공/완료 상태
			if e.Kind == "system" || e.Kind == "plan" {
				iconColor = m.theme.SystemTitleColor
			} else {
				iconColor = m.theme.UserTitleColor // 보통 녹색 계열
			}
		}

		iconView := m.theme.style(iconColor).Render(glyph)
		lines := strings.Split(rendered, "\n")
		// 첫 줄에만 아이콘 prefix를 붙인다.
		if len(lines) > 0 {
			lines[0] = iconView + " " + lines[0]
		}
		rendered = strings.Join(lines, "\n")
	default:
		// 기타 entry는 2칸 좌측 들여쓰기.
		lines := strings.Split(rendered, "\n")
		for i, line := range lines {
			lines[i] = "  " + line
		}
		rendered = strings.Join(lines, "\n")
	}

	return rendered
}

func (m uiModel) renderEntry(e transcriptEntry, streamingAssistant, streamingThinking, streamingTool, stalled bool) string {
	if e.Interactive != nil {
		return e.Interactive.View()
	}

	var rendered string

	body := e.Body
	if streamingThinking && m.thinkingFlushedLines > 0 {
		lines := strings.Split(e.Body, "\n")
		if m.thinkingFlushedLines < len(lines) {
			body = strings.Join(lines[m.thinkingFlushedLines:], "\n")
		} else {
			body = ""
		}
	}

	trimmedBody := strings.TrimSpace(body)
	if trimmedBody == "" && !streamingAssistant && !streamingThinking && !streamingTool {
		trimmedBody = "-"
	}

	termW := m.width - 5
	if termW < 20 {
		termW = 20
	}

	switch e.Kind {
	case "tool_group":
		rendered = m.renderToolGroup(e, termW)

	case "parallel_group":
		rendered = m.renderParallelGroup(e, termW)

	case "user":
		return m.renderUserEntry(trimmedBody, m.width)

	case "assistant":
		source := trimmedBody
		if streamingAssistant {
			source = m.assistantStreamSource(body)
			if m.assistantFlushedLines > 0 {
				lines := strings.Split(source, "\n")
				if m.assistantFlushedLines < len(lines) {
					source = strings.Join(lines[m.assistantFlushedLines:], "\n")
				} else {
					source = ""
				}
			}
		}
		source = strings.TrimLeft(source, "\n\r ")
		renderedMarkdown := strings.TrimLeft(markdown.Render(source, termW, m.themeToPalette()), "\n")
		if renderedMarkdown == "" && streamingAssistant {
			return ""
		}
		return m.renderAssistantEntry(renderedMarkdown, termW)

	case "thinking":
		return ""

	default:
		// 도구 전용 렌더러 우선 적용
		if e.ToolName != "" {
			if renderer := toolui.GetRenderer(e.ToolName); renderer != nil {
				switch e.Kind {
				case "tool_use":
					var args map[string]any
					if err := json.Unmarshal([]byte(e.Body), &args); err == nil {
						if args == nil {
							args = make(map[string]any)
						}
						args["_tool_name"] = e.ToolName
						args["_active_workspace"] = m.footer.Workspace
						rendered = renderer.RenderToolUse(args, e.StreamBody, m)
					}
				case "tool_result":
					rendered = renderer.RenderToolResult(e.Body, 0, m)
				}
				if rendered != "" {
					break
				}
				if e.Kind == "tool_result" {
					break
				}
			}
		}

		// TUI 모드 폴백 렌더링
		if !m.cfg.AIDebug {
			switch e.Kind {
			case "tool_use":
				name := toolui.NormalizeToolName(e.ToolName)
				hint := extractHint(e.ToolName, e.Body)
				if e.BashRepeatCount > 0 {
					attempts := fmt.Sprintf(" × %d attempts", e.BashRepeatCount+1)
					if !e.IsActive {
						if e.Failed {
							attempts += " → ✗"
						} else {
							attempts += " → ✓"
						}
					}
					hint += attempts
				}
				rendered = toolui.FormatToolCall(name, hint, 160, m)
				if e.IsActive {
					rendered += m.theme.style(m.theme.MutedColor).Render(m.animatedDots())
					rendered += "\n" + toolui.FormatStatusLine("Running"+m.animatedDots(), m)
				}
			case "tool_result":
				body := strings.TrimSpace(e.Body)
				if body == "" {
					rendered = toolui.FormatStatusLine("(no output)", m)
				} else {
					lines := strings.Split(body, "\n")
					for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
						lines = lines[:len(lines)-1]
					}
					const maxResultLines = 3
					if !m.transcriptMode && len(lines) > maxResultLines {
						rendered = toolui.FormatResultLines(lines[:maxResultLines], true, false, m)
						rendered += "\n" + toolui.FormatHiddenLinesHint(len(lines)-maxResultLines, m)
					} else {
						rendered = toolui.FormatResultLines(lines, true, false, m)
					}
				}
				// 텍스트 출력 후 하단에 "Done" 상태 표시 (claude_cli 패턴)
				if !e.IsActive {
					rendered += "\n" + toolui.FormatStatusLine("Done", m)
				}
			case "plan":
				if strings.Contains(e.Title, "Result") {
					titleColor := m.titleColorFor(e.Kind)
					rendered = m.theme.style(titleColor).Bold(true).Render(strings.TrimSpace(e.Title))
				}
			}
			if rendered != "" {
				break
			}
		}

		title := strings.TrimSpace(e.Title)
		// system 메시지는 제목 없이 본문만 표시
		if e.Kind == "system" || title == "" {
			title = ""
		}

		titleColor := m.titleColorFor(e.Kind)
		titleView := m.theme.style(titleColor).Bold(true).Render(title)
		bodyStyle := m.theme.style(m.theme.BodyColor)

		displayBody := body
		if streamingTool && e.StreamBody != "" {
			displayBody += "\n" + e.StreamBody
		}

		wrappedBody := strings.Join(markdown.WrapStyled(bodyStyle.Render(displayBody), termW), "\n")
		if title == "" {
			rendered = wrappedBody
		} else {
			rendered = lipgloss.JoinVertical(lipgloss.Left, titleView, wrappedBody)
		}
	}

	if rendered == "" {
		return ""
	}

	return m.padMultiline(rendered)
}

func (m uiModel) padMultiline(s string) string {
	width := m.width - 3
	if width <= 0 {
		width = 80
	}
	lines := strings.Split(s, "\n")
	for i := range lines {
		sw := lipgloss.Width(lines[i])
		if sw < width {
			lines[i] += strings.Repeat(" ", width-sw)
		}
	}
	return strings.Join(lines, "\n")
}

func (m uiModel) Style(color string) lipgloss.Style { return m.theme.style(color) }
func (m uiModel) BaseBodyStyle() lipgloss.Style { return m.theme.style(m.theme.BodyColor) }
func (m uiModel) Width() int { return m.width - 3 }
func (m uiModel) BodyColor() string { return m.theme.BodyColor }
func (m uiModel) MutedColor() string { return m.theme.MutedColor }
func (m uiModel) BorderColor() string { return m.theme.ModalBorderColor }
func (m uiModel) ToolUseColor() string { return m.theme.ToolUseTitleColor }
func (m uiModel) ToolResultColor() string { return m.theme.ToolResultTitleColor }
func (m uiModel) StatusSuccessColor() string { return m.theme.UserTitleColor }
func (m uiModel) StatusWarningColor() string { return m.theme.ApprovalTitleColor }
func (m uiModel) StatusErrorColor() string { return m.theme.ErrorTitleColor }
func (m uiModel) ClaudeAccentColor() string {
	if m.theme.ClaudeAccentColor != "" {
		return m.theme.ClaudeAccentColor
	}
	return m.theme.ApprovalTitleColor
}
func (m uiModel) AnimFrame() int { return m.animFrame }

func (m uiModel) titleColorFor(kind string) string {
	switch kind {
	case "tool_use":
		return m.theme.ToolUseTitleColor
	case "tool_result":
		return m.theme.ToolResultTitleColor
	case "notice":
		return m.theme.NoticeTitleColor
	case "system":
		return m.theme.SystemTitleColor
	case "error":
		return m.theme.ErrorTitleColor
	case "plan":
		return m.theme.PlanTitleColor
	case "approval", "password", "question":
		return m.theme.ApprovalTitleColor
	default:
		return m.theme.BodyColor
	}
}

func (m uiModel) renderUserEntry(body string, termW int) string {
	bgColor := m.theme.InputBoxBg
	hasBg := strings.TrimSpace(bgColor) != ""
	if !hasBg {
		bgColor = "#262626"
	}
	bg := lipgloss.Color(bgColor)

	if m.theme.DisableANSI {
		return "\n> " + wordwrap.String(body, termW) + "\n"
	}

	safeWidth := termW
	if safeWidth > 1 {
		safeWidth--
	}

	markerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.PromptTheme.PromptIndicatorColor)).Bold(true)
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.PromptTheme.PromptTextColor))
	lineStyle := lipgloss.NewStyle()

	if hasBg {
		markerStyle = markerStyle.Background(bg)
		textStyle = textStyle.Background(bg)
		lineStyle = lineStyle.Background(bg)
	}

	plainBody := stripANSI(body)
	wrappedBody := wordwrap.String(plainBody, safeWidth-4)
	lines := strings.Split(wrappedBody, "\n")
	formatted := make([]string, 0, len(lines)+2)

	topBorderStyle := lipgloss.NewStyle()
	if hasBg {
		topBorderStyle = topBorderStyle.Foreground(bg)
	} else {
		topBorderStyle = topBorderStyle.Foreground(lipgloss.Color(m.theme.PromptTheme.PromptIndicatorColor))
	}
	formatted = append(formatted, topBorderStyle.Render(strings.Repeat("▄", safeWidth)))

	for i, line := range lines {
		prefix := "  "
		if i == 0 {
			prefix = "> "
		}
		lineContent := prefix + line
		padding := safeWidth - lipgloss.Width(lineContent)
		res := markerStyle.Render(prefix) + textStyle.Render(line)
		if padding > 0 {
			res += lineStyle.Render(strings.Repeat(" ", padding))
		}
		formatted = append(formatted, res)
	}

	bottomBorderStyle := lipgloss.NewStyle()
	if hasBg {
		bottomBorderStyle = bottomBorderStyle.Foreground(bg)
	} else {
		bottomBorderStyle = bottomBorderStyle.Foreground(lipgloss.Color(m.theme.PromptTheme.PromptIndicatorColor))
	}
	formatted = append(formatted, bottomBorderStyle.Render(strings.Repeat("▀", safeWidth)))

	return "\n" + strings.Join(formatted, "\n")
}

var ansiRegex = regexp.MustCompile("[\u001b\u009b][[()#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-7,A-ORZcf-nqry=><]")

func stripANSI(str string) string {
	return ansiRegex.ReplaceAllString(str, "")
}

func (m uiModel) renderAssistantEntry(body string, termW int) string {
	if m.theme.DisableANSI {
		return "  " + body
	}
	lines := strings.Split(body, "\n")
	out := make([]string, len(lines))
	width := termW
	if width <= 0 {
		width = 80
	}
	for i, line := range lines {
		content := "  " + line
		if !markdown.IsBoxDrawingLine(stripANSI(line)) {
			currentWidth := lipgloss.Width(content)
			if currentWidth < width {
				content += strings.Repeat(" ", width-currentWidth)
			}
		}
		out[i] = content
	}
	return strings.Join(out, "\n")
}

func (m uiModel) themeToPalette() markdown.Palette {
	return markdown.Palette{
		Body:         m.theme.BodyColor,
		InlineCode:   m.theme.MarkdownInlineCodeColor,
		InlineCodeBg: m.theme.MarkdownInlineCodeBg,
		Link:         m.theme.MarkdownLinkColor,
		Strike:      m.theme.MarkdownStrikeColor,
		Heading1:    m.theme.MarkdownHeading1Color,
		Heading2:    m.theme.MarkdownHeading2Color,
		Heading3:    m.theme.MarkdownHeading3Color,
		Heading4:    m.theme.MarkdownHeading4Color,
		Quote:       m.theme.MarkdownQuoteColor,
		Rule:        m.theme.MarkdownRuleColor,
		CodeHead:    m.theme.MarkdownCodeHeadColor,
		CodeLine:    m.theme.MarkdownCodeLineColor,
		TableBorder: m.theme.MarkdownTableBorderColor,
		TableHeader: m.theme.MarkdownTableHeaderColor,
		TableRow:    m.theme.MarkdownTableRowColor,
		DisableANSI: m.theme.DisableANSI,
	}
}

func (m uiModel) assistantStreamSource(body string) string {
	mode := strings.ToLower(strings.TrimSpace(m.ui.Streaming.Mode))
	if mode == "" {
		mode = DefaultStreamingMode
	}
	switch mode {
	case "token-live":
		if m.ui.Streaming.HideUnstableMarkdown {
			return stableStreamingMarkdownSource(body)
		}
		return body
	case "line-stable":
		src := stableStreamingMarkdownSource(body)
		if !strings.HasSuffix(src, "\n") && strings.Contains(src, "\n") {
			lastNL := strings.LastIndex(src, "\n")
			src = src[:lastNL]
		}
		return src
	default:
		src := body
		if m.ui.Streaming.RenderCodeBlocksStable && hasOpenCodeFence(src) {
			src = stableStreamingMarkdownSource(src)
		} else if m.ui.Streaming.HideUnstableMarkdown && needsStableTail(src) {
			src = stableStreamingMarkdownSource(src)
		}
		if !m.ui.Streaming.FlushPlainTextPartial {
			src = stableStreamingMarkdownSource(src)
		}
		return src
	}
}

func hasOpenCodeFence(text string) bool { return strings.Count(text, "```")%2 == 1 || strings.Count(text, "~~~")%2 == 1 }
func needsStableTail(text string) bool {
	lastNL := strings.LastIndex(text, "\n")
	tail := text
	if lastNL >= 0 {
		tail = text[lastNL+1:]
	}
	trimmed := strings.TrimSpace(tail)
	if trimmed == "" {
		return false
	}
	if isUnstableMarkdownLine(tail) {
		return true
	}
	if strings.ContainsAny(trimmed, "#*`[]|>") {
		return true
	}
	return strings.Contains(trimmed, "http://") || strings.Contains(trimmed, "https://")
}
func stableStreamingMarkdownSource(text string) string {
	if text == "" {
		return ""
	}
	lastNL := strings.LastIndex(text, "\n")
	if lastNL < 0 {
		if isUnstableMarkdownLine(text) {
			return ""
		}
		return text
	}
	tail := text[lastNL+1:]
	if isUnstableMarkdownLine(tail) {
		return text[:lastNL+1]
	}
	return text
}
func isUnstableMarkdownLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	switch trimmed {
	case "#", "##", "###", "####", ">", "-", "*", "+", "```", "~~~":
		return true
	}
	if strings.HasPrefix(trimmed, "#") && !strings.Contains(trimmed, " ") {
		return true
	}
	if strings.HasPrefix(trimmed, "- ") && strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")) == "" {
		return true
	}
	if strings.HasPrefix(trimmed, "* ") && strings.TrimSpace(strings.TrimPrefix(trimmed, "* ")) == "" {
		return true
	}
	if strings.HasPrefix(trimmed, "+ ") && strings.TrimSpace(strings.TrimPrefix(trimmed, "+ ")) == "" {
		return true
	}
	if hasOddTokenCount(line, "`") || hasOddTokenCount(line, "**") || hasOddTokenCount(line, "~~") {
		return true
	}
	return strings.Count(line, "[") > strings.Count(line, "]")
}
func hasOddTokenCount(line, token string) bool {
	if token == "" {
		return false
	}
	return strings.Count(line, token)%2 == 1
}

func (m uiModel) isStreamStalled() bool {
	if !m.busy {
		return false
	}
	last := m.busyStartedAt
	if !m.assistantLastDelta.IsZero() && m.assistantLastDelta.After(last) {
		last = m.assistantLastDelta
	}
	if !m.thinkingLastDelta.IsZero() && m.thinkingLastDelta.After(last) {
		last = m.thinkingLastDelta
	}
	if last.IsZero() {
		return false
	}
	return time.Since(last) >= stalledAfter
}

func (m uiModel) renderTaskListRow() string {
	if len(m.latestTodos) == 0 {
		return ""
	}

	var lines []string
	for _, todo := range m.latestTodos {
		var icon string
		var color string
		switch todo.Status {
		case types.TodoStatusCompleted:
			icon = "●"
			color = m.theme.MutedColor
		case types.TodoStatusInProgress:
			icon = "◐"
			color = m.theme.ThinkingTitleColor
		default:
			icon = "○"
			color = m.theme.MutedColor
		}

		style := m.theme.style(color)
		line := fmt.Sprintf("  %s %s", style.Render(icon), todo.Content)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (m uiModel) renderThinkingRow() string {
	if !m.busy || m.busyStartedAt.IsZero() {
		return ""
	}
	sec := int(time.Since(m.busyStartedAt).Seconds())
	frame := IconThinking
	if m.motionEnabled() {
		if m.ui.Motion.Reduced || m.ui.Motion.Level == "restrained" {
			frame = asciiSpinnerFrames[m.motionFrame(len(asciiSpinnerFrames), 2)]
		} else {
			frame = spinnerFrames[m.motionFrame(len(spinnerFrames), 1)]
		}
	}
	frameColor := m.theme.ToolUseTitleColor
	if m.signatureMotionEnabled() {
		frameColor = m.getRainbowColor()
	}
	var verb string
	switch m.activeTokenKind {
	case "thinking":
		verb = "Thinking"
	case "output":
		verb = "Responding"
	case "input":
		verb = "Sketching"
	default:
		verb = m.spinnerVerb
		if verb == "" {
			verb = "Thinking"
		}
	}
	verbView := m.theme.style(frameColor).Bold(true).Italic(true).Render(verb)
	rainbowFrame := m.theme.style(frameColor).Bold(true).Render(frame)
	elapsed := formatElapsed(sec)
	tokenPart := ""
	if active := m.formatActiveToken(); active != "" {
		tokenPart = ", " + active
	}
	dots := m.animatedDots()
	leftText := fmt.Sprintf("%s %s%s (%s%s)", rainbowFrame, verbView, dots, elapsed, tokenPart)
	rightText := "? for shortcuts"
	if m.isStreamStalled() {
		leftText = fmt.Sprintf("%s %s", rainbowFrame, m.theme.style(m.theme.StatusLeftStalledColor).Render("Processing..."))
	}
	return renderTwoColumnLine(leftText, rightText, m.width, lipgloss.NewStyle(), m.theme.style(m.theme.StatusRightColor))
}

func (m uiModel) renderModeRow() string { return lipgloss.JoinVertical(lipgloss.Left, m.renderModeDivider(), m.renderModeStatus()) }
func (m uiModel) renderModeDivider() string { return m.theme.style(m.theme.MutedColor).Render(strings.Repeat("─", m.width)) }
func (m uiModel) renderModeStatus() string {
	left, leftColor := m.renderModeHintWithColor()
	right := m.renderContextSummary()
	gauge := m.renderContextGauge()
	return renderTwoColumnLine(left, right+"  "+gauge, m.width, m.theme.style(leftColor).Bold(true), m.theme.style(m.theme.StatusRightColor))
}
func (m uiModel) renderContextGauge() string {
	percent := m.footer.ContextUsedPercent
	if percent < 0 { percent = 0 }
	if percent > 100 { percent = 100 }
	width := 10
	filled := (percent * width) / 100
	gaugeColor := m.theme.StatusLeftColor
	if percent > 80 {
		gaugeColor = m.theme.ErrorTitleColor
	} else if percent > 50 {
		gaugeColor = m.theme.ApprovalTitleColor
	}
	fillGlyph, emptyGlyph := "█", "░"
	if m.theme.DisableANSI {
		fillGlyph, emptyGlyph = "#", "."
	}
	bar := m.theme.style(gaugeColor).Render(strings.Repeat(fillGlyph, filled)) + m.theme.style(m.theme.MutedColor).Render(strings.Repeat(emptyGlyph, width-filled))
	return fmt.Sprintf("[%s] %d%%", bar, percent)
}
func (m uiModel) renderModeHintWithColor() (string, string) {
	mode := types.PermissionMode(strings.TrimSpace(m.footer.PermissionMode))
	switch mode {
	case types.PermissionModeBypassPermissions: return "YOLO  Shift+Tab", m.theme.ModeYOLOColor
	case types.PermissionModeDontAsk, types.PermissionModeAcceptEdits: return "auto-accept edits  Shift+Tab", m.theme.ModeAutoColor
	case types.PermissionModePlan: return "plan  Shift+Tab", m.theme.ModePlanColor
	default: return "Shift+Tab to accept edits", m.theme.StatusRightColor
	}
}
func (m uiModel) renderContextSummary() string {
	parts := make([]string, 0, 3)
	if m.footer.MCPCount > 0 { parts = append(parts, fmt.Sprintf("%d MCP", m.footer.MCPCount)) }
	if m.footer.SkillCount > 0 { parts = append(parts, fmt.Sprintf("%d skill", m.footer.SkillCount)) }
	if m.footer.TodoCount > 0 { parts = append(parts, fmt.Sprintf("%d todo", m.footer.TodoCount)) }
	return strings.Join(parts, " | ")
}

func renderTwoColumnLine(left, right string, width int, leftStyle, rightStyle lipgloss.Style) string {
	left, right = leftStyle.Render(strings.TrimSpace(left)), rightStyle.Render(strings.TrimSpace(right))
	if width <= 0 { return left + "  " + right }
	if strings.TrimSpace(right) == "" { return padLine(truncateToWidth(left, width), width) }
	rw := lipgloss.Width(right)
	maxLeft := width - rw - 2
	if maxLeft < 4 { maxLeft = 4 }
	left = truncateToWidth(left, maxLeft)
	lw := lipgloss.Width(left)
	gap := width - lw - rw
	if gap < 0 { gap = 0 }
	return truncateToWidth(left+strings.Repeat(" ", gap)+right, width)
}
func padLine(s string, width int) string {
	sw := lipgloss.Width(s)
	if sw >= width { return s }
	return s + strings.Repeat(" ", width-sw)
}
func truncateToWidth(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width { return s }
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes)+"...") > width {
		runes = runes[:len(runes)-1]
	}
	if len(runes) == 0 { return "" }
	return string(runes) + "..."
}

func (m uiModel) renderInput() string {
	t := m.theme.PromptTheme
	if types.PermissionMode(m.footer.PermissionMode) == types.PermissionModeBypassPermissions {
		t.IndicatorChar, t.IndicatorColor = "*", m.theme.ModeYOLOColor
	}
	return promptinput.RenderTextInputWithTheme(m.input.View(), m.width, t)
}

func (m uiModel) renderModal() string {
	titleStyle := m.theme.style(m.theme.ModalTitleColor).Bold(true)
	borderStyle := m.theme.AccentBarStyle
	accentStyle := m.theme.ChevronStyle
	successStyle := m.theme.style(m.theme.UserTitleColor)
	mutedStyle := m.theme.style(m.theme.MutedColor)

	switch m.modal.Kind {
	case modalApproval, modalConfirm:
		inner := []string{titleStyle.Render("⦿ " + m.modal.Title), m.theme.style(m.theme.BodyColor).Render(m.modal.Prompt), ""}
		if m.modal.Kind == modalApproval {
			inner = append(inner, mutedStyle.Render("tool: ")+m.theme.style(m.theme.BodyColor).Render(m.modal.ToolName))
			switch m.modal.ToolType {
			case "edit":
				if m.modal.DiffContent != "" { inner = append(inner, "", toolui.RenderDiff(m.modal.DiffContent, m.width-8, m)) }
			case "exec":
				if len(m.modal.Commands) > 0 {
					for _, cmd := range m.modal.Commands { inner = append(inner, "", toolui.RenderCode(cmd, "bash", m.width-8, m)) }
				}
			default:
				if hint := approvalHint(m.modal.ToolName); hint != "" { inner = append(inner, mutedStyle.Render("hint: "+hint)) }
				if strings.TrimSpace(m.modal.InputJSON) != "" {
					maxW := m.width - 10
					if maxW < 20 { maxW = 20 }
					for _, jsonLine := range strings.Split(m.modal.InputJSON, "\n") { inner = append(inner, truncateToWidth(jsonLine, maxW)) }
				}
			}
			inner = append(inner, "")
		}
		options, hintText := []string{"Allow once", "Allow for this session", "Allow for all future sessions", "Deny"}, "↑↓ move  1-4 select  Enter confirm  y/n quick select  Esc deny"
		if m.modal.Kind == modalConfirm {
			options, hintText = []string{"Yes", "No"}, "↑↓ move  Enter select  y/n quick select  Esc deny"
		}
		for i, opt := range options {
			if i == m.modal.AskCursor { inner = append(inner, accentStyle.Render(fmt.Sprintf("› %d. %s", i+1, opt)))
			} else { inner = append(inner, m.theme.style(m.theme.BodyColor).Render(fmt.Sprintf("  %d. %s", i+1, opt))) }
		}
		inner = append(inner, "", mutedStyle.Render(hintText))
		return borderStyle.Render(strings.Join(inner, "\n"))
	case modalPassword:
		mask := strings.Repeat("*", len(m.modal.Password))
		return borderStyle.Render(strings.Join([]string{titleStyle.Render("⦿ " + m.modal.Title), strings.TrimSpace(m.modal.Prompt) + " " + mask + "█", "", mutedStyle.Render("Enter submit  Esc cancel")}, "\n"))
	case modalAskUser:
		if m.modal.Question == nil { return borderStyle.Render(strings.Join([]string{titleStyle.Render("⦿ Question"), "(missing question payload)", "", "Esc cancel"}, "\n")) }
		q := m.modal.Question
		header := strings.TrimSpace(q.Header)
		if header == "" { header = m.modal.Title; if strings.TrimSpace(header) == "" { header = "Question" } }
		inner := []string{titleStyle.Render("⦿ " + header)}
		if previewContent := strings.TrimSpace(q.Preview); previewContent != "" {
			termW := m.width - 10
			if termW < 20 { termW = 20 }
			renderedPreview := markdown.Render(previewContent, termW-4, m.themeToPalette())
			inner = append(inner, lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(m.theme.ModalBorderColor)).Padding(0, 1).Width(termW).Render(strings.TrimRight(renderedPreview, "\n")), "")
		}
		inner = append(inner, strings.TrimSpace(q.Question), "")
		qType := normalizeAskQuestionType(q)
		switch qType {
		case "text":
			value := m.modal.AskInput
			if strings.TrimSpace(value) == "" { value = strings.TrimSpace(q.Placeholder); if value == "" { value = "(type your answer)" } }
			inner = append(inner, value, "", mutedStyle.Render("Type answer  Enter submit  Esc cancel"))
		default:
			options := askQuestionOptions(q)
			if len(options) == 0 { inner = append(inner, "(no options)")
			} else {
				for i, opt := range options {
					isFocused, isChecked := i == m.modal.AskCursor, q.MultiSelect && m.modal.AskSelected != nil && m.modal.AskSelected[i]
					label := strings.TrimSpace(opt.Label)
					if label == "" { label = strings.TrimSpace(opt.Value) }
					var line string
					if q.MultiSelect {
						box := "◯"; if isChecked { box = successStyle.Render("◉") }
						line = fmt.Sprintf("%s %s", box, label)
					} else {
						check := ""; if isChecked { check = successStyle.Render(" [selected]") }
						line = fmt.Sprintf("  %s%s", label, check)
					}
					if isFocused { line = accentStyle.Render("› " + line[2:]) } else { line = "  " + line[2:] }
					if desc := strings.TrimSpace(opt.Description); desc != "" { line += " " + mutedStyle.Render("- "+desc) }
					inner = append(inner, line)
				}
			}
			inner = append(inner, "")
			if q.MultiSelect { inner = append(inner, mutedStyle.Render("↑↓ move  Space toggle  Enter submit  Esc cancel"))
			} else { inner = append(inner, mutedStyle.Render("↑↓ move  1-9 select  Enter submit  Esc cancel")) }
		}
		return borderStyle.Render(strings.Join(inner, "\n"))
	case modalAskUserBatch:
		if len(m.modal.Questions) == 0 { return borderStyle.Render(strings.Join([]string{titleStyle.Render("Questions"), "(missing question payload)", "", "[Esc] cancel"}, "\n")) }
		stepInfo := fmt.Sprintf("Step %d of %d", m.modal.AskTab+1, len(m.modal.Questions))
		if m.isAskBatchReviewTab() { stepInfo = "Review" }
		inner := []string{titleStyle.Render(m.modal.Title + " - " + stepInfo)}
		tabs := make([]string, 0, len(m.modal.Questions)+1)
		for i, q := range m.modal.Questions {
			header := strings.TrimSpace(q.Header)
			if header == "" {
				header = fmt.Sprintf("Q%d", i+1)
			}
			answered := strings.TrimSpace(m.modal.AskAnswersByIndex[strconv.Itoa(i)]) != ""

			label := header
			if answered {
				label += " *"
			}
			if i == m.modal.AskTab { tabs = append(tabs, accentStyle.Bold(true).Render("["+label+"]")) } else { tabs = append(tabs, mutedStyle.Render("["+label+"]")) }
		}
		if len(m.modal.Questions) > 1 {
			reviewIdx, label := len(m.modal.Questions), "Review"
			if reviewIdx == m.modal.AskTab { tabs = append(tabs, accentStyle.Bold(true).Render("["+label+"]")) } else { tabs = append(tabs, mutedStyle.Render("["+label+"]")) }
		}
		inner = append(inner, strings.Join(tabs, " "), "")
		if len(m.modal.Questions) > 1 && m.modal.AskTab == len(m.modal.Questions) {
			inner = append(inner, "Review your answers:")
			unanswered := 0
			for i, q := range m.modal.Questions {
				value, header := strings.TrimSpace(m.modal.AskAnswersByIndex[strconv.Itoa(i)]), strings.TrimSpace(q.Header)
				if value == "" { value = strings.TrimSpace(q.Default) }
				if header == "" { header = fmt.Sprintf("Q%d", i+1) }
				if value == "" { unanswered++; inner = append(inner, mutedStyle.Render(fmt.Sprintf("- %s: (not answered)", header))) } else { inner = append(inner, fmt.Sprintf("- %s: %s", header, value)) }
			}
			inner = append(inner, "")
			if unanswered > 0 { inner = append(inner, mutedStyle.Render(fmt.Sprintf("%d unanswered questions", unanswered))) }
			if strings.TrimSpace(m.modal.AskError) != "" { inner = append(inner, m.theme.style(m.theme.ErrorTitleColor).Render(m.modal.AskError)) }
			inner = append(inner, mutedStyle.Render("[Tab/Shift+Tab] switch  [Enter] submit  [Esc] cancel"))
			return borderStyle.Render(strings.Join(inner, "\n"))
		}
		q := m.modal.Questions[m.modal.AskTab]
		if previewContent := strings.TrimSpace(q.Preview); previewContent != "" {
			termW := m.width - 10
			if termW < 20 { termW = 20 }
			renderedPreview := markdown.Render(previewContent, termW-4, m.themeToPalette())
			inner = append(inner, lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(m.theme.ModalBorderColor)).Padding(0, 1).Width(termW).Render(strings.TrimRight(renderedPreview, "\n")), "")
		}
		inner = append(inner, strings.TrimSpace(q.Question), "")
		switch normalizeAskQuestionType(&q) {
		case "text":
			value := m.modal.AskInput
			if strings.TrimSpace(value) == "" { value = strings.TrimSpace(q.Placeholder); if value == "" { value = "(type your answer)" } }
			inner = append(inner, value, "")
			if strings.TrimSpace(m.modal.AskError) != "" { inner = append(inner, m.theme.style(m.theme.ErrorTitleColor).Render(m.modal.AskError)) }
			msg := "[Type answer] [Enter] next  [Tab] switch  [Esc] cancel"; if len(m.modal.Questions) == 1 { msg = "[Type answer] [Enter] submit  [Esc] cancel" }
			inner = append(inner, mutedStyle.Render(msg))
		default:
			options := askQuestionOptions(&q)
			if len(options) == 0 { inner = append(inner, "(no options)")
			} else {
				for i, opt := range options {
					isFocused, checked := i == m.modal.AskCursor, false
					if q.MultiSelect { checked = m.modal.AskSelected != nil && m.modal.AskSelected[i]
					} else { current := strings.TrimSpace(m.modal.AskAnswersByIndex[strconv.Itoa(m.modal.AskTab)]); checked = strings.EqualFold(current, optionAnswerValue(opt)) || strings.EqualFold(current, strings.TrimSpace(opt.Label)) }
					label := strings.TrimSpace(opt.Label); if label == "" { label = strings.TrimSpace(opt.Value) }
					line := "  " + label; if q.MultiSelect { box := "◯"; if checked { box = successStyle.Render("◉") }; line = box + " " + label } else if checked { line = label + " [selected]" }
					if isFocused { line = accentStyle.Bold(true).Render("> " + line) } else { line = "  " + line }
					if desc := strings.TrimSpace(opt.Description); desc != "" { line += " " + mutedStyle.Render("- "+desc) }
					inner = append(inner, line)
				}
			}
			inner = append(inner, "")
			if strings.TrimSpace(m.modal.AskError) != "" { inner = append(inner, m.theme.style(m.theme.ErrorTitleColor).Render(m.modal.AskError)) }
			msg := "[Up/Down] move  [1-9] select  [Enter] next  [Tab] switch  [Esc] cancel"; if q.MultiSelect { msg = "[Up/Down] move  [Space] toggle  [Enter] next  [Tab] switch  [Esc] cancel" }
			inner = append(inner, mutedStyle.Render(msg))
		}
		return borderStyle.Render(strings.Join(inner, "\n"))
	case modalServerForm: return m.renderServerForm(titleStyle, accentStyle, mutedStyle, borderStyle)
	case modalServerList: return m.renderServerList(titleStyle, accentStyle, mutedStyle, borderStyle)
	case modalModelList: return borderStyle.Render(strings.Join([]string{titleStyle.Render("⦿ " + m.modal.Title), "", m.renderModelList()}, "\n"))
	case modalConnectorSearch: return m.renderConnectorSearch(titleStyle, mutedStyle, borderStyle)
	case modalConnectorInstall: return m.renderConnectorInstall(titleStyle, mutedStyle, borderStyle)
	default: return ""
	}
}

func formatDiff(added, removed int) string { if added == 0 && removed == 0 { return "-" }; return fmt.Sprintf("+%d -%d", added, removed) }
func formatElapsed(sec int) string { if sec >= 60 { return fmt.Sprintf("%dm %ds", sec/60, sec%60) }; return fmt.Sprintf("%ds", sec) }
func formatTokenCount(n int) string { if n >= 1_000_000 { return fmt.Sprintf("%.1fm", float64(n)/1_000_000) }; if n >= 1_000 { return fmt.Sprintf("%.1fk", float64(n)/1_000) }; return fmt.Sprintf("%d", n) }
func (m uiModel) formatActiveToken() string {
	switch m.activeTokenKind {
	case "thinking": if m.tokenThinkingSnap > 0 { return constants.EffortMedium + formatTokenCount(m.tokenThinkingSnap) }
	case "output": if m.tokenOutputSnap > 0 { return constants.DownArrow + formatTokenCount(m.tokenOutputSnap) }
	case "input": if m.tokenInputSnap > 0 { return constants.UpArrow + formatTokenCount(m.tokenInputSnap) }
	}
	switch {
	case m.tokenThinkingSnap > 0: return constants.EffortMedium + formatTokenCount(m.tokenThinkingSnap)
	case m.tokenOutputSnap > 0: return constants.DownArrow + formatTokenCount(m.tokenOutputSnap)
	case m.tokenInputSnap > 0: return constants.UpArrow + formatTokenCount(m.tokenInputSnap)
	}
	return ""
}
func formatTokensDetailed(in, out, thinking int) string {
	if in == 0 && out == 0 && thinking == 0 { return "-" }
	parts := make([]string, 0, 3)
	if in > 0 { parts = append(parts, constants.UpArrow+formatTokenCount(in)) }
	if out > 0 { parts = append(parts, constants.DownArrow+formatTokenCount(out)) }
	if thinking > 0 { parts = append(parts, constants.EffortMedium+formatTokenCount(thinking)) }
	if len(parts) == 0 { return "-" }; return strings.Join(parts, " ")
}
func emptyFallback(v, fallback string) string { if strings.TrimSpace(v) == "" { return fallback }; return strings.TrimSpace(v) }
func approvalHint(toolName string) string {
	n := strings.ToLower(toolName)
	switch {
	case strings.Contains(n, "search"): return "Review source URLs before continuing."
	case strings.Contains(n, "fetch"), strings.Contains(n, "crawl"): return "This may pull full page content."
	case strings.Contains(n, "bash"), strings.Contains(n, "shell"), strings.Contains(n, "powershell"): return "Command execution may modify files or system state."
	case strings.Contains(n, "write"), strings.Contains(n, "edit"), strings.Contains(n, "patch"): return "Verify target path and changes before allowing."
	default: return ""
	}
}

func renderLogoBlock(cfg Config) string {
	var modelDisplay, effortLevel, providerName string
	effortLevel = "high"; if cfg.State != nil { _, disp, _ := cfg.State.ActiveModel(); modelDisplay, effortLevel, providerName = disp, cfg.State.GetEffortLevel(), cfg.State.ActiveModelProvider() }
	return logo.Render(logo.Data{Version: constants.Version, ModelDisplay: modelDisplay, EffortLevel: effortLevel, ProviderName: providerName, Cwd: cfg.WorkDir, SourcePath: constants.ConfigDirName + "\\" + constants.SettingsFileName}, cfg.AIDebug)
}

func (m uiModel) renderToolGroup(e transcriptEntry, _ int) string {
	name := "Research"; if e.SearchCount > 0 { name = "WebSearch" }
	headText := name; if e.LastHint != "" { headText = fmt.Sprintf("%s(%s)", name, e.LastHint) }
	parts := make([]string, 0, 2)
	if e.SearchCount > 0 {
		if e.ReadCount > 0 { parts = append(parts, fmt.Sprintf("%d read", e.ReadCount)) }
		if e.ListCount > 0 { parts = append(parts, fmt.Sprintf("%d list", e.ListCount)) }
	} else {
		if e.ReadCount > 0 { parts = append(parts, fmt.Sprintf("%d read", e.ReadCount)) }
		if e.ListCount > 0 { parts = append(parts, fmt.Sprintf("%d list", e.ListCount)) }
	}
	if len(parts) > 0 { headText += " · " + strings.Join(parts, ", ") }
	if e.IsActive {
		if m.motionEnabled() {
			frame := spinnerFrames[m.motionFrame(len(spinnerFrames), 1)]
			if m.ui.Motion.Reduced || m.ui.Motion.Level == "restrained" {
				frame = asciiSpinnerFrames[m.motionFrame(len(asciiSpinnerFrames), 2)]
			}
			headText += " " + frame + m.animatedDots()
		} else {
			headText += "..."
		}
	}
	headerColor := m.theme.ToolUseTitleColor; if !e.IsActive { headerColor = m.theme.StatusLeftColor }
	header, collapsed := m.theme.style(headerColor).Render(headText), e.Collapsed && !m.transcriptMode
	if collapsed && !e.IsActive { header += m.theme.style(m.theme.MutedColor).Render(" (ctrl+o to expand)")
	} else if !collapsed && !e.IsActive { header += m.theme.style(m.theme.MutedColor).Render(" (ctrl+o to collapse)") }
	var sb strings.Builder; sb.WriteString(header)
	if !collapsed {
		for _, sub := range e.SubEntries {
			rendered := m.renderEntry(sub, false, false, false, false)
			if rendered == "" { continue }
			// 하위 항목도 ● 아이콘과 함께 ⎿ 브랜치로 연결
			subRendered := m.renderTranscriptEntryAt(-1, sub)
			lines := strings.Split(subRendered, "\n")
			for i, line := range lines {
				prefix := "    "
				if i == 0 { prefix = "  ⎿ " }
				lines[i] = prefix + line
			}
			sb.WriteString("\n" + strings.Join(lines, "\n"))
		}
	}
	return sb.String()
}

func (m uiModel) renderParallelGroup(e transcriptEntry, _ int) string {
	n := len(e.SubEntries); if n == 0 { return "" }
	failedCount := 0; for _, sub := range e.SubEntries { if sub.Failed { failedCount++ } }
	accentStyle, bodyStyle, mutedStyle, errorStyle := m.theme.style(m.ClaudeAccentColor()), m.theme.style(m.theme.BodyColor), m.theme.style(m.theme.MutedColor), m.theme.style(m.theme.ErrorTitleColor)
	glyph, header := accentStyle.Render("⇌"), ""
	if e.IsActive {
		if m.motionEnabled() {
			frame := spinnerFrames[m.motionFrame(len(spinnerFrames), 1)]
			if m.ui.Motion.Reduced || m.ui.Motion.Level == "restrained" {
				frame = asciiSpinnerFrames[m.motionFrame(len(asciiSpinnerFrames), 2)]
			}
			glyph = accentStyle.Render(frame)
		}
		header = fmt.Sprintf("%s %s%s", glyph, bodyStyle.Render(fmt.Sprintf("%d개 병렬 실행 중", n)), m.theme.style(m.theme.MutedColor).Render(m.animatedDots()))
	} else {
		duration := ""; if !e.StartTime.IsZero() && !e.EndTime.IsZero() { d := e.EndTime.Sub(e.StartTime); if d < time.Second { duration = fmt.Sprintf(" (%dms)", d.Milliseconds()) } else { duration = fmt.Sprintf(" (%.1fs)", d.Seconds()) } }
		base := bodyStyle.Render(fmt.Sprintf("%d개 병렬 완료", n)) + mutedStyle.Render(duration)
		if failedCount > 0 { header = fmt.Sprintf("%s %s — %s", glyph, base, errorStyle.Render(fmt.Sprintf("%d개 실패", failedCount))) } else { header = fmt.Sprintf("%s %s", glyph, base) }
	}
	var sb strings.Builder; sb.WriteString(header)
	for _, sub := range e.SubEntries {
		rendered := m.renderTranscriptEntryAt(-1, sub)
		if rendered == "" { continue }
		lines := strings.Split(rendered, "\n")
		for i, line := range lines {
			prefix := "    "
			if i == 0 { prefix = "  ⎿ " }
			sb.WriteString("\n" + prefix + line)
		}
	}
	return sb.String()
}
