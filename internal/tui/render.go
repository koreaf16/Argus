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

const (
	IconSuccess    = "✓"
	IconPending    = "○"
	IconExecuting  = "⚙"
	IconThinking   = "⦿"
	IconConfirming = "?"
	IconCanceled   = "-"
	IconError      = "✕"
)

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

func (m uiModel) renderGlimmerText(text string, stalled bool) string {
	if text == "" || m.theme.DisableANSI {
		return m.theme.style(m.theme.BodyColor).Render(text)
	}
	if !m.signatureMotionEnabled() {
		color := m.theme.BodyColor
		if stalled {
			color = m.theme.StalledColor
		}
		return m.theme.style(color).Render(text)
	}
	colors := m.theme.GlimmerTrailColors
	if len(colors) == 0 {
		colors = []string{m.theme.BodyColor}
	}
	runes := []rune(text)
	if len(runes) <= 6 {
		color := m.theme.BodyColor
		if stalled {
			color = m.theme.StalledColor
		}
		return m.theme.style(color).Render(text)
	}

	head := string(runes[:len(runes)-6])
	tail := runes[len(runes)-6:]
	var buf strings.Builder
	buf.WriteString(m.theme.style(m.theme.BodyColor).Render(head))

	// Rainbow glimmer tail
	for i, r := range tail {
		colorIdx := (m.motionFrame(len(colors), 2) + i) % len(colors)
		color := colors[colorIdx]
		if stalled {
			color = m.theme.StalledColor
		}
		buf.WriteString(m.theme.style(color).Render(string(r)))
	}
	return buf.String()
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

	// 모든 출력 앞에 기본 패딩 제거 (왼쪽으로 2칸 이동 효과)
	const indent = ""

	// 아이콘 영역 패딩 초기화 (0칸)
	padding := ""

	if idx >= 0 {
		var icon string
		var iconColor string

		switch e.Kind {
		case "error":
			icon = IconError
			iconColor = m.theme.ErrorTitleColor
		case "thinking":
			icon = IconThinking
			iconColor = m.theme.ThinkingTitleColor
		case "tool_use":
			if e.IsActive {
				icon = IconExecuting
				iconColor = m.theme.StatusLeftColor
			} else {
				icon = IconPending
				iconColor = m.theme.ToolUseTitleColor
			}
		case "tool_result":
			icon = IconSuccess
			iconColor = m.theme.StatusLeftColor
		}

		if icon != "" {
			iconView := m.theme.style(iconColor).Bold(true).Render(icon)
			padding = " " + iconView + " "
		}
	}

	// 아이콘이나 패딩이 있을 때만 적용
	if padding != "" {
		lines := strings.Split(rendered, "\n")
		prefix := strings.Repeat(" ", lipgloss.Width(padding))
		for i, line := range lines {
			if i == 0 {
				lines[i] = padding + line
			} else {
				lines[i] = prefix + line
			}
		}
		rendered = strings.Join(lines, "\n")
	}

	// 모든 줄 끝에 ANSI 이스케이프 시퀀스로 잔상 제거 (\x1b[K) 제거 (깜빡임 방지)
	return rendered
}

func (m uiModel) renderEntry(e transcriptEntry, streamingAssistant, streamingThinking, streamingTool, stalled bool) string {
	// 동적 상호작용 모델이 있으면 그 View()를 우선 사용 (앞에 2칸 패딩 추가)
	if e.Interactive != nil {
		rendered := e.Interactive.View()
		lines := strings.Split(rendered, "\n")
		for i, line := range lines {
			lines[i] = "  " + line
		}
		return strings.Join(lines, "\n")
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

	// 아이콘 영역(3칸)을 제외한 가용 너비 계산
	termW := m.width - 3
	if termW < 20 {
		termW = 20
	}

	switch e.Kind {
	case "tool_group":
		rendered = m.renderToolGroup(e, termW)

	case "user":
		// user entry already handles padding internally, use full width
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
		renderedMarkdown := markdown.Render(source, termW, m.themeToPalette())
		if renderedMarkdown == "" && streamingAssistant {
			return ""
		}
		// assistant entry already handles padding internally
		return m.renderAssistantEntry(renderedMarkdown, termW)

	case "thinking":
		title := strings.TrimSpace(e.Title)
		if title == "" {
			title = "Thinking"
		}

		borderStyle := m.theme.AccentBarStyle

		titleView := m.theme.style(m.theme.ThinkingTitleColor).Bold(true).Italic(true).Render("⦿ " + title)
		bodyStyle := m.theme.style(m.theme.BodyColor).Italic(true)

		var content string
		if streamingThinking && !m.theme.DisableANSI {
			content = m.renderGlimmerText(body, stalled)
		} else {
			content = bodyStyle.Render(trimmedBody)
		}

		var inner string
		if titleView == "" {
			inner = borderStyle.Render(wordwrap.String(content, termW-2))
		} else {
			inner = borderStyle.Render(titleView + "\n" + wordwrap.String(content, termW-2))
		}
		rendered = m.padMultiline(inner)

	default:
		// 도구 전용 렌더러가 있으면 AIDebug 여부와 관계없이 항상 우선 적용
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
			}
		}

		// TUI 모드(AIDebug=false)에서는 커스텀 렌더러 없는 도구의 상세 정보와 플랜 결과를 숨김
		if !m.cfg.AIDebug {
			switch e.Kind {
			case "tool_use", "tool_result":
				title := strings.TrimSpace(e.Title)
				if title == "" {
					title = "-"
				}
				titleColor := m.titleColorFor(e.Kind)
				titleView := m.theme.style(titleColor).Bold(true).Render(title)

				if e.Kind == "tool_result" && strings.TrimSpace(e.Body) != "" {
					body := strings.TrimSpace(e.Body)
					// 결과가 너무 길면 첫 줄만 표시
					firstLine := strings.Split(body, "\n")[0]
					if len(body) > len(firstLine) {
						firstLine += " ..."
					}
					rendered = titleView + " " + m.theme.style(m.theme.BodyColor).Render(firstLine)
				} else {
					rendered = titleView
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
		if title == "" {
			title = "-"
		}
		titleColor := m.titleColorFor(e.Kind)
		titleView := m.theme.style(titleColor).Bold(true).Render(title)
		bodyStyle := m.theme.style(m.theme.BodyColor)

		displayBody := body
		if streamingTool && e.StreamBody != "" {
			displayBody += "\n" + e.StreamBody
		}

		rendered = lipgloss.JoinVertical(lipgloss.Left, titleView, wordwrap.String(bodyStyle.Render(displayBody), termW))
	}

	if rendered == "" {
		return ""
	}

	return m.padMultiline(rendered)
}

func (m uiModel) padMultiline(s string) string {
	// 아이콘 영역(3칸)을 제외한 가용 너비 사용
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

func (m uiModel) Style(color string) lipgloss.Style {
	return m.theme.style(color)
}

func (m uiModel) BaseBodyStyle() lipgloss.Style {
	return m.theme.style(m.theme.BodyColor)
}

func (m uiModel) Width() int {
	return m.width - 3
}

func (m uiModel) BodyColor() string {
	return m.theme.BodyColor
}

func (m uiModel) MutedColor() string {
	return m.theme.MutedColor
}

func (m uiModel) BorderColor() string {
	return m.theme.ModalBorderColor
}

func (m uiModel) ToolUseColor() string {
	return m.theme.ToolUseTitleColor
}

func (m uiModel) ToolResultColor() string {
	return m.theme.ToolResultTitleColor
}

func (m uiModel) StatusSuccessColor() string {
	return m.theme.UserTitleColor
}

func (m uiModel) StatusWarningColor() string {
	return m.theme.ApprovalTitleColor
}

func (m uiModel) StatusErrorColor() string { return m.theme.ErrorTitleColor }
func (m uiModel) AnimFrame() int           { return m.animFrame }

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

// renderUserEntry renders a user message with the same integrated box style as the input prompt.
func (m uiModel) renderUserEntry(body string, termW int) string {
	bgColor := m.theme.InputBoxBg
	if strings.TrimSpace(bgColor) == "" {
		bgColor = "#262626"
	}
	bg := lipgloss.Color(bgColor)
	if m.theme.DisableANSI {
		return "> " + wordwrap.String(body, termW)
	}

	markerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.PromptTheme.PromptIndicatorColor)).
		Background(bg).
		Bold(true)
	textStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.PromptTheme.PromptTextColor)).
		Background(bg)
	lineStyle := lipgloss.NewStyle().Background(bg)

	plainBody := stripANSI(body)
	// termW-6 gives enough room for the "> " and padding
	wrappedBody := wordwrap.String(plainBody, termW-6)
	lines := strings.Split(wrappedBody, "\n")

	// Use the provided width to ensure it fits within the terminal after parent indentation
	width := termW
	if width <= 0 {
		width = 80
	}

	formatted := make([]string, 0, len(lines))
	for i, line := range lines {
		prefix := "  "
		if i == 0 {
			prefix = "> "
		}

		// Calculate padding before rendering to avoid ANSI width issues
		// 줄 시작의 공백 제거하여 왼쪽으로 한 칸 당김
		lineContent := prefix + line
		padding := width - lipgloss.Width(lineContent)

		// Render parts separately and combine to ensure background persists
		res := markerStyle.Render(prefix) + textStyle.Render(line)
		if padding > 0 {
			res += lineStyle.Render(strings.Repeat(" ", padding))
		}
		formatted = append(formatted, res)
	}

	topStyle := lipgloss.NewStyle().Foreground(bg)
	bottomStyle := lipgloss.NewStyle().Foreground(bg)

	topBorder := topStyle.Render(strings.Repeat("▄", width))
	bottomBorder := bottomStyle.Render(strings.Repeat("▀", width))

	return strings.Join([]string{topBorder, strings.Join(formatted, "\n"), bottomBorder}, "\n")
}

var ansiRegex = regexp.MustCompile("[\u001b\u009b][[()#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-7,A-ORZcf-nqry=><]")

func stripANSI(str string) string {
	return ansiRegex.ReplaceAllString(str, "")
}

func (m uiModel) renderAssistantEntry(body string, termW int) string {
	if m.theme.DisableANSI {
		return body
	}
	marker := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.AssistantMarkerColor)).
		Render("✦ ")
	lines := strings.Split(body, "\n")
	out := make([]string, len(lines))
	for i, line := range lines {
		var content string
		if i == 0 {
			content = marker + line
		} else {
			content = "  " + line
		}

		// Pad to termW to clear any residual text
		width := termW
		if width <= 0 {
			width = 80
		}
		currentWidth := lipgloss.Width(content)
		if currentWidth < width {
			content += strings.Repeat(" ", width-currentWidth)
		}
		out[i] = content
	}
	return strings.Join(out, "\n")
}

// themeToPalette builds a markdown.Palette from the current uiTheme.
func (m uiModel) themeToPalette() markdown.Palette {
	return markdown.Palette{
		Body:        m.theme.BodyColor,
		InlineCode:  m.theme.MarkdownInlineCodeColor,
		Link:        m.theme.MarkdownLinkColor,
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
	default: // hybrid-stable
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

func hasOpenCodeFence(text string) bool {
	return strings.Count(text, "```")%2 == 1 || strings.Count(text, "~~~")%2 == 1
}

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
	if strings.HasPrefix(trimmed, "- ") && strings.TrimSpace(trimmed[2:]) == "" {
		return true
	}
	if strings.HasPrefix(trimmed, "* ") && strings.TrimSpace(trimmed[2:]) == "" {
		return true
	}
	if strings.HasPrefix(trimmed, "+ ") && strings.TrimSpace(trimmed[2:]) == "" {
		return true
	}
	if hasOddTokenCount(line, "`") {
		return true
	}
	if hasOddTokenCount(line, "**") {
		return true
	}
	if hasOddTokenCount(line, "~~") {
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

func (m uiModel) renderThinkingRow() string {
	if !m.busy || m.busyStartedAt.IsZero() {
		return ""
	}
	sec := int(time.Since(m.busyStartedAt).Seconds())

	frame := IconThinking
	if len(m.theme.SpinnerFrames) > 0 {
		frame = m.theme.SpinnerFrames[m.motionFrame(len(m.theme.SpinnerFrames), 2)]
	}
	if !m.motionEnabled() {
		frame = IconThinking
	}
	frameColor := m.theme.ToolUseTitleColor
	if m.signatureMotionEnabled() {
		frameColor = m.getRainbowColor()
	}
	rainbowFrame := m.theme.style(frameColor).Bold(true).Render(frame)

	verb := m.spinnerVerb
	if verb == "" {
		verb = "Thinking"
	}

	leftText := fmt.Sprintf("%s %s... (%ds)", rainbowFrame, verb, sec)
	rightText := "? for shortcuts"

	// 델타가 오랫동안 없더라도 'Thinking'은 유지하여 멈춘 느낌 방지
	if m.isStreamStalled() {
		leftText = fmt.Sprintf("%s %s", rainbowFrame, m.theme.style(m.theme.StatusLeftStalledColor).Render("Processing..."))
	}

	return renderTwoColumnLine(leftText, rightText, m.width,
		lipgloss.NewStyle(),
		m.theme.style(m.theme.StatusRightColor))
}

func (m uiModel) renderModeRow() string {
	return lipgloss.JoinVertical(lipgloss.Left, m.renderModeDivider(), m.renderModeStatus())
}

func (m uiModel) renderModeDivider() string {
	return m.theme.style(m.theme.MutedColor).Render(strings.Repeat("─", m.width))
}

func (m uiModel) renderModeStatus() string {
	left, leftColor := m.renderModeHintWithColor()
	right := m.renderContextSummary()
	gauge := m.renderContextGauge()

	return renderTwoColumnLine(left, right+"  "+gauge, m.width,
		m.theme.style(leftColor).Bold(true),
		m.theme.style(m.theme.StatusRightColor))
}

func (m uiModel) renderContextGauge() string {
	percent := m.footer.ContextUsedPercent
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	width := 10
	filled := (percent * width) / 100

	gaugeColor := m.theme.StatusLeftColor
	if percent > 80 {
		gaugeColor = m.theme.ErrorTitleColor
	} else if percent > 50 {
		gaugeColor = m.theme.ApprovalTitleColor
	}

	fillGlyph := "█"
	emptyGlyph := "░"
	if m.theme.DisableANSI {
		fillGlyph = "#"
		emptyGlyph = "."
	}

	bar := m.theme.style(gaugeColor).Render(strings.Repeat(fillGlyph, filled)) +
		m.theme.style(m.theme.MutedColor).Render(strings.Repeat(emptyGlyph, width-filled))

	return fmt.Sprintf("[%s] %d%%", bar, percent)
}

func (m uiModel) renderModeHintWithColor() (string, string) {
	mode := types.PermissionMode(strings.TrimSpace(m.footer.PermissionMode))
	switch mode {
	case types.PermissionModeBypassPermissions:
		return "YOLO  Shift+Tab", m.theme.ModeYOLOColor
	case types.PermissionModeDontAsk, types.PermissionModeAcceptEdits:
		return "auto-accept edits  Shift+Tab", m.theme.ModeAutoColor
	case types.PermissionModePlan:
		return "plan  Shift+Tab", m.theme.ModePlanColor
	default:
		return "Shift+Tab to accept edits", m.theme.StatusRightColor
	}
}

func (m uiModel) renderContextSummary() string {
	parts := make([]string, 0, 3)
	if m.footer.MCPCount > 0 {
		parts = append(parts, fmt.Sprintf("%d MCP", m.footer.MCPCount))
	}
	if m.footer.SkillCount > 0 {
		parts = append(parts, fmt.Sprintf("%d skill", m.footer.SkillCount))
	}
	if m.footer.TodoCount > 0 {
		parts = append(parts, fmt.Sprintf("%d todo", m.footer.TodoCount))
	}
	return strings.Join(parts, " | ")
}

func renderTwoColumnLine(left, right string, width int, leftStyle, rightStyle lipgloss.Style) string {
	left = leftStyle.Render(strings.TrimSpace(left))
	right = rightStyle.Render(strings.TrimSpace(right))
	if width <= 0 {
		if strings.TrimSpace(right) == "" {
			return left
		}
		return left + "  " + right
	}
	if strings.TrimSpace(right) == "" {
		return padLine(truncateToWidth(left, width), width)
	}

	rw := lipgloss.Width(right)
	maxLeft := width - rw - 2
	if maxLeft < 4 {
		maxLeft = 4
	}

	left = truncateToWidth(left, maxLeft)
	lw := lipgloss.Width(left)

	gap := width - lw - rw
	if gap < 0 {
		gap = 0
	}
	return left + strings.Repeat(" ", gap) + right
}

func padLine(s string, width int) string {
	if width <= 0 {
		return s
	}
	sw := lipgloss.Width(s)
	if sw >= width {
		return s
	}
	return s + strings.Repeat(" ", width-sw)
}

func truncateToWidth(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes)+"...") > width {
		runes = runes[:len(runes)-1]
	}
	if len(runes) == 0 {
		return ""
	}
	return string(runes) + "..."
}

func (m uiModel) renderInput() string {
	t := m.theme.PromptTheme
	if types.PermissionMode(m.footer.PermissionMode) == types.PermissionModeBypassPermissions {
		t.IndicatorChar = "*"
		t.IndicatorColor = m.theme.ModeYOLOColor
	}

	return promptinput.RenderTextInputWithTheme(m.input.View(), m.width, t)
}

func (m uiModel) renderModal() string {
	titleStyle := m.theme.style(m.theme.ModalTitleColor).Bold(true)

	// 디자인 시스템 v2.1: 전체 테두리 대신 좌측 액센트 바만 사용
	borderStyle := m.theme.AccentBarStyle

	accentStyle := m.theme.ChevronStyle
	successStyle := m.theme.style(m.theme.UserTitleColor)
	mutedStyle := m.theme.style(m.theme.MutedColor)

	switch m.modal.Kind {
	case modalApproval, modalConfirm:
		inner := []string{
			titleStyle.Render("⦿ " + m.modal.Title),
			m.theme.style(m.theme.BodyColor).Render(m.modal.Prompt),
			"",
		}
		if m.modal.Kind == modalApproval {
			inner = append(inner, mutedStyle.Render("tool: ")+m.theme.style(m.theme.BodyColor).Render(m.modal.ToolName))

			// Specialized Rendering based on ToolType
			switch m.modal.ToolType {
			case "edit":
				if m.modal.DiffContent != "" {
					inner = append(inner, "", toolui.RenderDiff(m.modal.DiffContent, m.width-8, m))
				}
			case "exec":
				if len(m.modal.Commands) > 0 {
					for _, cmd := range m.modal.Commands {
						inner = append(inner, "", toolui.RenderCode(cmd, "bash", m.width-8, m))
					}
				}
			default:
				if hint := approvalHint(m.modal.ToolName); hint != "" {
					inner = append(inner, mutedStyle.Render("hint: "+hint))
				}
				if strings.TrimSpace(m.modal.InputJSON) != "" {
					maxW := m.width - 10
					if maxW < 20 {
						maxW = 20
					}
					for _, jsonLine := range strings.Split(m.modal.InputJSON, "\n") {
						inner = append(inner, truncateToWidth(jsonLine, maxW))
					}
				}
			}
			inner = append(inner, "")
		}

		options := []string{"Allow once", "Allow for this session", "Allow for all future sessions", "Deny"}
		hintText := "↑↓ move  1-4 select  Enter confirm  y/n quick select  Esc deny"
		if m.modal.Kind == modalConfirm {
			options = []string{"Yes", "No"}
			hintText = "↑↓ move  Enter select  y/n quick select  Esc deny"
		}

		for i, opt := range options {
			var line string
			if i == m.modal.AskCursor {
				line = accentStyle.Render(fmt.Sprintf("› %d. %s", i+1, opt))
			} else {
				line = m.theme.style(m.theme.BodyColor).Render(fmt.Sprintf("  %d. %s", i+1, opt))
			}
			inner = append(inner, line)
		}

		inner = append(inner, "", mutedStyle.Render(hintText))
		return borderStyle.Render(strings.Join(inner, "\n"))

	case modalPassword:
		mask := strings.Repeat("*", len(m.modal.Password))
		inner := []string{
			titleStyle.Render("⦿ " + m.modal.Title),
			strings.TrimSpace(m.modal.Prompt) + " " + mask + "█",
			"",
			mutedStyle.Render("Enter submit  Esc cancel"),
		}
		return borderStyle.Render(strings.Join(inner, "\n"))

	case modalAskUser:
		if m.modal.Question == nil {
			return borderStyle.Render(strings.Join([]string{
				titleStyle.Render("⦿ Question"),
				"(missing question payload)",
				"",
				"Esc cancel",
			}, "\n"))
		}

		q := m.modal.Question
		header := strings.TrimSpace(q.Header)
		if header == "" {
			header = m.modal.Title
			if strings.TrimSpace(header) == "" {
				header = "Question"
			}
		}
		inner := []string{titleStyle.Render("⦿ " + header), strings.TrimSpace(q.Question), ""}

		qType := normalizeAskQuestionType(q)
		switch qType {
		case "text":
			value := m.modal.AskInput
			if strings.TrimSpace(value) == "" {
				value = strings.TrimSpace(q.Placeholder)
				if value == "" {
					value = "(type your answer)"
				}
			}
			inner = append(inner, value, "")
			inner = append(inner, mutedStyle.Render("Type answer  Enter submit  Esc cancel"))
		default:
			options := askQuestionOptions(q)
			if len(options) == 0 {
				inner = append(inner, "(no options)")
			} else {
				for i, opt := range options {
					isFocused := i == m.modal.AskCursor
					isChecked := false
					if q.MultiSelect {
						if m.modal.AskSelected != nil && m.modal.AskSelected[i] {
							isChecked = true
						}
					}

					label := strings.TrimSpace(opt.Label)
					if label == "" {
						label = strings.TrimSpace(opt.Value)
					}

					var line string
					if q.MultiSelect {
						box := "[ ]"
						if isChecked {
							box = successStyle.Render("[x]")
						}
						line = fmt.Sprintf("%s %s", box, label)
					} else {
						check := ""
						if isChecked {
							check = successStyle.Render(" [selected]")
						}
						line = fmt.Sprintf("  %s%s", label, check)
					}

					if isFocused {
						line = accentStyle.Render("› " + line[2:])
					} else {
						line = "  " + line[2:]
					}

					if desc := strings.TrimSpace(opt.Description); desc != "" {
						line += " " + mutedStyle.Render("- "+desc)
					}
					inner = append(inner, line)
				}
			}
			inner = append(inner, "")
			if q.MultiSelect {
				inner = append(inner, mutedStyle.Render("↑↓ move  Space toggle  Enter submit  Esc cancel"))
			} else {
				inner = append(inner, mutedStyle.Render("↑↓ move  1-9 select  Enter submit  Esc cancel"))
			}
		}
		return borderStyle.Render(strings.Join(inner, "\n"))

	case modalAskUserBatch:
		if len(m.modal.Questions) == 0 {
			return borderStyle.Render(strings.Join([]string{
				titleStyle.Render("Questions"),
				"(missing question payload)",
				"",
				"[Esc] cancel",
			}, "\n"))
		}

		inner := []string{titleStyle.Render(m.modal.Title)}
		tabs := make([]string, 0, len(m.modal.Questions)+1)
		for i, q := range m.modal.Questions {
			header := strings.TrimSpace(q.Header)
			if header == "" {
				header = fmt.Sprintf("Q%d", i+1)
			}
			idx := strconv.Itoa(i)
			answered := strings.TrimSpace(m.modal.AskAnswersByIndex[idx]) != ""
			label := header
			if answered {
				label += " *"
			}
			if i == m.modal.AskTab {
				tabs = append(tabs, accentStyle.Bold(true).Render("["+label+"]"))
			} else {
				tabs = append(tabs, mutedStyle.Render("["+label+"]"))
			}
		}
		if len(m.modal.Questions) > 1 {
			reviewIdx := len(m.modal.Questions)
			label := "Review"
			if reviewIdx == m.modal.AskTab {
				tabs = append(tabs, accentStyle.Bold(true).Render("["+label+"]"))
			} else {
				tabs = append(tabs, mutedStyle.Render("["+label+"]"))
			}
		}
		inner = append(inner, strings.Join(tabs, " "), "")

		isReview := len(m.modal.Questions) > 1 && m.modal.AskTab == len(m.modal.Questions)
		if isReview {
			inner = append(inner, "Review your answers:")
			unanswered := 0
			for i, q := range m.modal.Questions {
				idx := strconv.Itoa(i)
				value := strings.TrimSpace(m.modal.AskAnswersByIndex[idx])
				if value == "" {
					value = strings.TrimSpace(q.Default)
				}
				header := strings.TrimSpace(q.Header)
				if header == "" {
					header = fmt.Sprintf("Q%d", i+1)
				}
				if value == "" {
					unanswered++
					inner = append(inner, mutedStyle.Render(fmt.Sprintf("- %s: (not answered)", header)))
				} else {
					inner = append(inner, fmt.Sprintf("- %s: %s", header, value))
				}
			}
			inner = append(inner, "")
			if unanswered > 0 {
				inner = append(inner, mutedStyle.Render(fmt.Sprintf("%d unanswered questions", unanswered)))
			}
			if strings.TrimSpace(m.modal.AskError) != "" {
				inner = append(inner, m.theme.style(m.theme.ErrorTitleColor).Render(m.modal.AskError))
			}
			inner = append(inner, mutedStyle.Render("[Tab/Shift+Tab] switch  [Enter] submit  [Esc] cancel"))
			return borderStyle.Render(strings.Join(inner, "\n"))
		}

		if m.modal.AskTab < 0 || m.modal.AskTab >= len(m.modal.Questions) {
			inner = append(inner, "(invalid question index)")
			return borderStyle.Render(strings.Join(inner, "\n"))
		}
		q := m.modal.Questions[m.modal.AskTab]
		inner = append(inner, strings.TrimSpace(q.Question), "")

		qType := normalizeAskQuestionType(&q)
		switch qType {
		case "text":
			value := m.modal.AskInput
			if strings.TrimSpace(value) == "" {
				value = strings.TrimSpace(q.Placeholder)
				if value == "" {
					value = "(type your answer)"
				}
			}
			inner = append(inner, value, "")
			if strings.TrimSpace(m.modal.AskError) != "" {
				inner = append(inner, m.theme.style(m.theme.ErrorTitleColor).Render(m.modal.AskError))
			}
			if len(m.modal.Questions) > 1 {
				inner = append(inner, mutedStyle.Render("[Type answer] [Enter] next  [Tab] switch  [Esc] cancel"))
			} else {
				inner = append(inner, mutedStyle.Render("[Type answer] [Enter] submit  [Esc] cancel"))
			}
		default:
			options := askQuestionOptions(&q)
			if len(options) == 0 {
				inner = append(inner, "(no options)")
			} else {
				for i, opt := range options {
					isFocused := i == m.modal.AskCursor
					checked := false
					if q.MultiSelect {
						checked = m.modal.AskSelected != nil && m.modal.AskSelected[i]
					} else {
						idx := strconv.Itoa(m.modal.AskTab)
						current := strings.TrimSpace(m.modal.AskAnswersByIndex[idx])
						checked = strings.EqualFold(current, optionAnswerValue(opt)) || strings.EqualFold(current, strings.TrimSpace(opt.Label))
					}

					label := strings.TrimSpace(opt.Label)
					if label == "" {
						label = strings.TrimSpace(opt.Value)
					}
					line := "  " + label
					if q.MultiSelect {
						prefix := "[ ]"
						if checked {
							prefix = "[x]"
						}
						line = prefix + " " + label
					} else if checked {
						line = label + " [selected]"
					}
					if isFocused {
						line = accentStyle.Bold(true).Render("> " + line)
					} else {
						line = "  " + line
					}
					if desc := strings.TrimSpace(opt.Description); desc != "" {
						line += " " + mutedStyle.Render("- "+desc)
					}
					inner = append(inner, line)
				}
			}
			inner = append(inner, "")
			if strings.TrimSpace(m.modal.AskError) != "" {
				inner = append(inner, m.theme.style(m.theme.ErrorTitleColor).Render(m.modal.AskError))
			}
			if q.MultiSelect {
				inner = append(inner, mutedStyle.Render("[Up/Down] move  [Space] toggle  [Enter] next  [Tab] switch  [Esc] cancel"))
			} else {
				inner = append(inner, mutedStyle.Render("[Up/Down] move  [1-9] select  [Enter] next  [Tab] switch  [Esc] cancel"))
			}
		}
		return borderStyle.Render(strings.Join(inner, "\n"))
	case modalServerForm:
		return m.renderServerForm(titleStyle, accentStyle, mutedStyle, borderStyle)
	case modalServerList:
		return m.renderServerList(titleStyle, accentStyle, mutedStyle, borderStyle)
	default:
		return ""
	}
}

func formatDiff(added, removed int) string {
	if added == 0 && removed == 0 {
		return "-"
	}
	return fmt.Sprintf("+%d -%d", added, removed)
}

func formatTokens(n int) string {
	if n == 0 {
		return "-"
	}
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fm tokens", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%dk tokens", n/1_000)
	}
	return fmt.Sprintf("%d tokens", n)
}

func emptyFallback(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return strings.TrimSpace(v)
}

func approvalHint(toolName string) string {
	n := strings.ToLower(toolName)
	switch {
	case strings.Contains(n, "search"):
		return "Review source URLs before continuing."
	case strings.Contains(n, "fetch"), strings.Contains(n, "crawl"):
		return "This may pull full page content."
	case strings.Contains(n, "bash"), strings.Contains(n, "shell"), strings.Contains(n, "powershell"):
		return "Command execution may modify files or system state."
	case strings.Contains(n, "write"), strings.Contains(n, "edit"), strings.Contains(n, "patch"):
		return "Verify target path and changes before allowing."
	default:
		return ""
	}
}

func renderLogoBlock(cfg Config) string {
	var modelDisplay, effortLevel, providerName string
	effortLevel = "high"
	if cfg.State != nil {
		_, disp, _ := cfg.State.ActiveModel()
		modelDisplay = disp
		effortLevel = cfg.State.GetEffortLevel()
		providerName = cfg.State.ActiveModelProvider()
	}
	return logo.Render(logo.Data{
		Version:      constants.Version,
		ModelDisplay: modelDisplay,
		EffortLevel:  effortLevel,
		ProviderName: providerName,
		Cwd:          cfg.WorkDir,
		SourcePath:   constants.ConfigDirName + "\\" + constants.SettingsFileName,
	}, cfg.AIDebug)
}

// getSearchReadSummaryText 는 tool_group 진행 요약 텍스트를 반환한다.
func getSearchReadSummaryText(searchCount, readCount, listCount int, isActive bool) string {
	parts := make([]string, 0, 3)
	if searchCount > 0 {
		parts = append(parts, fmt.Sprintf("%d search", searchCount))
	}
	if readCount > 0 {
		parts = append(parts, fmt.Sprintf("%d read", readCount))
	}
	if listCount > 0 {
		parts = append(parts, fmt.Sprintf("%d list", listCount))
	}

	suffix := ""
	if isActive {
		suffix = "…"
	}
	if len(parts) == 0 {
		if isActive {
			return "Working" + suffix
		}
		return "Done"
	}
	return strings.Join(parts, ", ") + suffix
}

func (m uiModel) renderToolGroup(e transcriptEntry, termW int) string {
	summaryText := getSearchReadSummaryText(e.SearchCount, e.ReadCount, e.ListCount, e.IsActive)

	var icon string
	var iconColor string
	if e.IsActive {
		// 애니메이션 스피너 프레임 선택
		frames := m.theme.SpinnerFrames
		if len(frames) > 0 {
			icon = frames[m.motionFrame(len(frames), 2)]
		} else {
			icon = IconThinking
		}
		iconColor = m.theme.ToolUseTitleColor
		if m.signatureMotionEnabled() {
			iconColor = m.getRainbowColor()
		}
	} else {
		icon = IconSuccess
		iconColor = m.theme.StatusLeftColor
	}

	iconView := m.theme.style(iconColor).Bold(true).Render(icon)
	mutedStyle := m.theme.style(m.theme.MutedColor)

	var sb strings.Builder
	// Design System v2.1: Header with spinner and modern spacing (2칸 들여쓰기 보장)
	header := "  " + iconView + " " + m.theme.style(m.theme.BodyColor).Bold(true).Render(summaryText)
	if e.Collapsed && !e.IsActive {
		header += mutedStyle.Render(" (ctrl+o)")
	}
	sb.WriteString(header)

	if e.LastHint != "" {
		sb.WriteString("\n    " + mutedStyle.Render("› "+e.LastHint))
	}

	if !e.Collapsed {
		borderStyle := m.theme.AccentBarStyle.BorderForeground(lipgloss.Color(m.theme.MutedColor)).Margin(0, 0)
		var subContent []string
		for _, sub := range e.SubEntries {
			rendered := m.renderEntry(sub, false, false, false, false)
			if rendered != "" {
				// 하위 항목들에 4칸 들여쓰기 적용
				lines := strings.Split(rendered, "\n")
				for i, line := range lines {
					lines[i] = "    " + line
				}
				subContent = append(subContent, strings.Join(lines, "\n"))
			}
		}
		if len(subContent) > 0 {
			sb.WriteString("\n" + borderStyle.Render(strings.Join(subContent, "\n")))
		}
	}

	return sb.String()
}
