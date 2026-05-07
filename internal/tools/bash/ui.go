package bash

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/koreaf16/argus/internal/tools/shellsignal"
	"github.com/koreaf16/argus/internal/tui/toolui"
)

var ansiRegex = regexp.MustCompile("[\u001B\u009B][[()#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-7,A-ORZcf-nqry=><]")

const (
	maxShellOutputBufferChars = 128 * 1024
	shellLiveTailLines        = 10
)

func stripANSI(str string) string {
	return ansiRegex.ReplaceAllString(str, "")
}

func init() {
	toolui.Register("bash", &BashRenderer{})
	toolui.Register("powershell", &BashRenderer{})
}

type shellExecResult struct {
	Stdout                    string
	Stderr                    string
	Code                      int
	BackgroundTaskID          string
	OutputTaskID              string
	AssistantAutoBackgrounded bool
	BackgroundedByUser        bool
}

// BashInteractiveModel renders streaming shell tools and handles stdin focus.
type BashInteractiveModel struct {
	toolName         string
	targetAlias      string
	showTarget       bool
	command          string
	description      string
	theme            toolui.ThemeContext
	spinner          spinner.Model
	output           string
	isFocused        bool
	isExpanded       bool
	isFinished       bool
	result           string
	inputChan        chan string
	shellGuard       toolui.ShellGuardInfo
	hasShellGuard    bool
	exitCode         int
	hasResult        bool
	resultStdout     string
	resultStderr     string
	backgroundTaskID string
	isBackground     bool
}

func (m *BashInteractiveModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m *BashInteractiveModel) Update(msg tea.Msg) (toolui.InteractiveModel, tea.Cmd) {
	switch v := msg.(type) {
	case tea.KeyMsg:
		if m.inputChan == nil || m.isFinished {
			break
		}
		if v.String() == "ctrl+d" {
			// focused 상태에선 EOF(0x04)를 stdin으로 보내고, 그렇지 않으면 백그라운드 전환 신호.
			payload := shellsignal.BackgroundRequest
			if m.isFocused {
				payload = "\x04"
			}
			select {
			case m.inputChan <- payload:
			default:
			}
			return m, nil
		}
		if !m.isFocused {
			break
		}

		s := v.String()
		switch s {
		case "enter":
			s = "\n"
		case "tab":
			s = "\t"
		case "ctrl+c":
			s = "\x03"
		default:
			if len(s) != 1 {
				return m, nil
			}
		}

		select {
		case m.inputChan <- s:
		default:
		}
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(v)
		return m, cmd
	}
	return m, nil
}

func collapseToSingleLine(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
}

func (m *BashInteractiveModel) View() string {
	displayName := toolui.NormalizeToolName(m.toolName)
	args := collapseToSingleLine(m.command)
	if m.showTarget {
		target := strings.TrimSpace(m.targetAlias)
		if target == "" {
			target = "local"
		}
		args = fmt.Sprintf("%s [%s]", args, target)
	}

	maxCmd := max(m.theme.Width()-15, 60)
	headline := toolui.FormatToolCall(displayName, args, maxCmd, m.theme)

	blocks := []string{headline}
	if guard := m.renderShellGuardPrelude(); guard != "" {
		blocks = append(blocks, guard)
		if m.shellGuard.BlocksExecution() {
			return strings.Join(blocks, "\n")
		}
	}

	if m.shouldRenderSummary() {
		blocks = append(blocks, m.renderSummaryLine())
		return strings.Join(blocks, "\n")
	}

	lines := m.getFilteredLines()
	visible := lines
	if !m.isExpanded && len(visible) > shellLiveTailLines {
		visible = visible[len(visible)-shellLiveTailLines:]
	}
	if len(visible) == 0 {
		visible = []string{"waiting for output..."}
	}

	boxLines := []string{m.renderBoxTop(m.liveStatusLine(len(lines), len(visible)), m.liveStatusColor())}
	for _, line := range visible {
		boxLines = append(boxLines, m.renderBoxLine(line, m.theme.BodyColor()))
	}
	boxLines = append(boxLines, m.renderBoxBottom(m.liveHintLine(len(lines)), m.theme.MutedColor(), true))
	blocks = append(blocks, strings.Join(boxLines, "\n"))
	return strings.Join(blocks, "\n")
}

func (m *BashInteractiveModel) renderShellGuardPrelude() string {
	if !m.hasShellGuard {
		return ""
	}
	return toolui.FormatShellGuardPrelude(m.shellGuard, m.command, m.targetAlias, m.theme)
}

func (m *BashInteractiveModel) shouldRenderSummary() bool {
	return m.isFinished || m.isBackground
}

func (m *BashInteractiveModel) liveStatusLine(totalLines, visibleLines int) string {
	status := "running"
	if m.isFocused {
		status = "focused | stdin attached"
	}
	if m.isExpanded {
		if totalLines > 0 {
			return fmt.Sprintf("%s | %d lines", status, totalLines)
		}
		return status
	}
	if totalLines > shellLiveTailLines {
		return fmt.Sprintf("%s | tail %d/%d", status, visibleLines, totalLines)
	}
	if totalLines > 0 {
		return fmt.Sprintf("%s | %d lines", status, totalLines)
	}
	return status
}

func (m *BashInteractiveModel) liveStatusColor() string {
	if m.isFocused {
		return m.theme.StatusWarningColor()
	}
	return m.theme.ToolUseColor()
}

func (m *BashInteractiveModel) liveHintLine(totalLines int) string {
	if m.isFocused {
		return "TAB/ESC release | Ctrl+D EOF | Ctrl+C interrupt"
	}

	parts := []string{"TAB focus", "Ctrl+D background"}
	if totalLines > shellLiveTailLines {
		if m.isExpanded {
			parts = append(parts, "Ctrl+O collapse")
		} else {
			parts = append(parts, "Ctrl+O expand")
		}
	}
	return strings.Join(parts, " | ")
}

func (m *BashInteractiveModel) renderSummaryLine() string {
	summary, color := m.summaryText()
	return m.renderBranchLine(summary, color, false)
}

func (m *BashInteractiveModel) summaryText() (string, string) {
	if m.backgroundTaskID != "" || m.isBackground {
		text := "background"
		if m.backgroundTaskID != "" {
			text += " | " + m.backgroundTaskID
		}
		if snippet := m.summarySnippet(true); snippet != "" {
			text += " | last: " + snippet
		}
		return text, m.theme.StatusWarningColor()
	}

	snippet := m.summarySnippet(false)
	countText := m.summaryLineCountText()

	if m.hasResult {
		if m.exitCode == 0 {
			text := "done | exit 0"
			if snippet != "" {
				text += " | " + snippet
			}
			if countText != "" {
				text += " | " + countText
			}
			return text, m.theme.StatusSuccessColor()
		}

		text := fmt.Sprintf("failed | exit %d", m.exitCode)
		if snippet != "" {
			text += " | " + snippet
		}
		if countText != "" {
			text += " | " + countText
		}
		return text, m.theme.StatusErrorColor()
	}

	if snippet == "" {
		snippet = "done"
	}
	return snippet, m.theme.StatusSuccessColor()
}

func (m *BashInteractiveModel) summarySnippet(background bool) string {
	if m.hasResult {
		if !background && m.exitCode != 0 {
			if line := firstNonEmptyLine(m.resultStderr); line != "" {
				return truncateSummaryLine(line)
			}
		}
		if line := lastNonEmptyLine(m.resultStdout); line != "" {
			return truncateSummaryLine(line)
		}
		if line := lastNonEmptyLine(m.resultStderr); line != "" {
			return truncateSummaryLine(line)
		}
	}

	lines := m.getFilteredLines()
	for i := len(lines) - 1; i >= 0; i-- {
		if trimmed := strings.TrimSpace(lines[i]); trimmed != "" {
			return truncateSummaryLine(trimmed)
		}
	}
	return ""
}

func (m *BashInteractiveModel) summaryLineCountText() string {
	count := len(m.getFilteredLines())
	if count == 0 && m.hasResult {
		count = max(nonEmptyLineCount(m.resultStdout), nonEmptyLineCount(m.resultStderr))
	}
	if count == 0 {
		return "no output"
	}
	if count == 1 {
		return "1 line"
	}
	return fmt.Sprintf("%d lines", count)
}

func (m *BashInteractiveModel) renderBranchLine(text, color string, italic bool) string {
	prefix := m.theme.Style(m.theme.MutedColor()).Render("  ⎿ ")
	style := m.theme.Style(color)
	if italic {
		style = style.Italic(true)
	}
	return prefix + style.Render(text)
}

func (m *BashInteractiveModel) liveBoxWidth() int {
	return max(m.theme.Width(), 44)
}

func (m *BashInteractiveModel) renderBoxTop(text, color string) string {
	w := m.liveBoxWidth()
	muted := m.theme.Style(m.theme.MutedColor())
	visLen := len([]rune(text))
	fillLen := max(w-7-visLen, 1) // "  ╭─ "(5) + " "(1) + "╮"(1)
	return muted.Render("  ╭─ ") +
		m.theme.Style(color).Render(text) +
		muted.Render(" "+strings.Repeat("─", fillLen)+"╮")
}

func (m *BashInteractiveModel) renderBoxLine(text, color string) string {
	w := m.liveBoxWidth()
	innerW := w - 6 // "  │ "(4) + " │"(2)
	muted := m.theme.Style(m.theme.MutedColor())

	cleanText := strings.ReplaceAll(text, "\r", "")
	runes := []rune(cleanText)
	visLen := len(runes)

	if visLen > innerW {
		if innerW > 3 {
			runes = runes[:innerW-3]
		} else {
			runes = runes[:0]
		}
		cleanText = string(runes) + "..."
		visLen = innerW
	}

	padding := strings.Repeat(" ", innerW-visLen)
	return muted.Render("  │ ") +
		m.theme.Style(color).Render(cleanText) +
		padding +
		muted.Render(" │")
}

func (m *BashInteractiveModel) renderBoxBottom(text, color string, italic bool) string {
	w := m.liveBoxWidth()
	muted := m.theme.Style(m.theme.MutedColor())
	visLen := len([]rune(text))
	fillLen := max(w-7-visLen, 1) // "  ╰─ "(5) + " "(1) + "╯"(1)
	style := m.theme.Style(color)
	if italic {
		style = style.Italic(true)
	}
	return muted.Render("  ╰─ ") +
		style.Render(text) +
		muted.Render(" "+strings.Repeat("─", fillLen)+"╯")
}

func (m *BashInteractiveModel) getFilteredLines() []string {
	s := strings.ReplaceAll(m.output, "\r\n", "\n")

	var processed strings.Builder
	var currentLine strings.Builder
	for _, r := range s {
		if r == '\r' {
			currentLine.Reset()
			continue
		}
		if r == '\n' {
			processed.WriteString(currentLine.String())
			processed.WriteRune('\n')
			currentLine.Reset()
			continue
		}
		currentLine.WriteRune(r)
	}
	processed.WriteString(currentLine.String())

	cleanOutput := stripANSI(processed.String())
	allLines := strings.Split(cleanOutput, "\n")
	filtered := make([]string, 0, len(allLines))
	for _, line := range allLines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "__ARG_M_") || strings.Contains(trimmed, "ARGUS_SU_PW") {
			continue
		}
		filtered = append(filtered, strings.TrimRight(line, " "))
	}

	for len(filtered) > 0 && strings.TrimSpace(filtered[0]) == "" {
		filtered = filtered[1:]
	}
	for len(filtered) > 0 && strings.TrimSpace(filtered[len(filtered)-1]) == "" {
		filtered = filtered[:len(filtered)-1]
	}
	return filtered
}

func (m *BashInteractiveModel) SetFocus(focus bool)       { m.isFocused = focus }
func (m *BashInteractiveModel) IsFocused() bool           { return m.isFocused }
func (m *BashInteractiveModel) SetExpanded(expanded bool) { m.isExpanded = expanded }
func (m *BashInteractiveModel) IsExpanded() bool          { return m.isExpanded }

func (m *BashInteractiveModel) OnStreamDelta(delta string) {
	m.output += delta
	if len(m.output) > maxShellOutputBufferChars {
		m.output = m.output[len(m.output)-maxShellOutputBufferChars:]
	}
}

func (m *BashInteractiveModel) SetInputResponse(input chan string) { m.inputChan = input }
func (m *BashInteractiveModel) SetFinished(finished bool)          { m.isFinished = finished }

func (m *BashInteractiveModel) SetResult(text string) {
	m.result = text

	if execResult, ok := parseShellExecResult(text); ok {
		m.hasResult = true
		m.exitCode = execResult.Code
		m.resultStdout = stripANSI(normalizeShellText(execResult.Stdout))
		m.resultStderr = stripANSI(normalizeShellText(execResult.Stderr))
		m.backgroundTaskID = firstNonEmptyValue(execResult.BackgroundTaskID, execResult.OutputTaskID)
		if m.backgroundTaskID != "" || execResult.AssistantAutoBackgrounded || execResult.BackgroundedByUser {
			m.isBackground = true
		}
		return
	}

	if code, ok := parseNormalizedExitCode(text); ok {
		m.hasResult = true
		m.exitCode = code
		m.resultStdout = stripANSI(normalizeShellText(normalizedSectionBody(text, "stdout")))
		m.resultStderr = stripANSI(normalizeShellText(normalizedSectionBody(text, "stderr")))
	}
}

// BashRenderer provides custom UI for bash and powershell tools.
type BashRenderer struct{}

func (r *BashRenderer) CreateInteractiveModel(args map[string]any, theme toolui.ThemeContext) toolui.InteractiveModel {
	command, _ := args["command"].(string)
	description, _ := args["description"].(string)
	toolName, _ := args["_tool_name"].(string)
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		toolName = "bash"
	}

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = theme.Style(theme.StatusWarningColor())

	shellGuard, hasShellGuard := toolui.ParseShellGuard(args)

	return &BashInteractiveModel{
		toolName:      toolName,
		targetAlias:   resolveTargetAlias(args),
		showTarget:    true,
		command:       command,
		description:   description,
		theme:         theme,
		spinner:       s,
		shellGuard:    shellGuard,
		hasShellGuard: hasShellGuard,
	}
}

func (r *BashRenderer) RenderToolUse(args map[string]any, streamBody string, theme toolui.ThemeContext) string {
	command, _ := args["command"].(string)
	toolName, _ := args["_tool_name"].(string)
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		toolName = "bash"
	}

	shellGuard, hasShellGuard := toolui.ParseShellGuard(args)

	s := &BashInteractiveModel{
		toolName:      toolName,
		targetAlias:   resolveTargetAlias(args),
		showTarget:    true,
		command:       collapseToSingleLine(command),
		theme:         theme,
		output:        streamBody,
		shellGuard:    shellGuard,
		hasShellGuard: hasShellGuard,
	}
	return s.View()
}

func (r *BashRenderer) RenderToolResult(_ string, _ int64, _ toolui.ThemeContext) string {
	// The interactive shell entry owns both the live body and the final summary.
	return ""
}

func parseShellExecResult(text string) (shellExecResult, bool) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(text), &raw); err != nil || len(raw) == 0 {
		return shellExecResult{}, false
	}

	hasAnyField := false
	result := shellExecResult{}

	if value := stringFromMap(raw, "stdout", "Stdout"); value != "" {
		result.Stdout = value
		hasAnyField = true
	}
	if value := stringFromMap(raw, "stderr", "Stderr"); value != "" {
		result.Stderr = value
		hasAnyField = true
	}
	if value, ok := intFromMap(raw, "code", "Code"); ok {
		result.Code = value
		hasAnyField = true
	}
	if value := stringFromMap(raw, "background_task_id", "BackgroundTaskID"); value != "" {
		result.BackgroundTaskID = value
		hasAnyField = true
	}
	if value := stringFromMap(raw, "output_task_id", "OutputTaskID"); value != "" {
		result.OutputTaskID = value
		hasAnyField = true
	}
	if value, ok := boolFromMap(raw, "assistant_auto_backgrounded", "AssistantAutoBackgrounded"); ok {
		result.AssistantAutoBackgrounded = value
		hasAnyField = true
	}
	if value, ok := boolFromMap(raw, "backgrounded_by_user", "BackgroundedByUser"); ok {
		result.BackgroundedByUser = value
		hasAnyField = true
	}

	return result, hasAnyField
}

func parseNormalizedExitCode(text string) (int, bool) {
	trimmed := strings.TrimSpace(text)
	const prefix = "exit_code:"
	if !strings.HasPrefix(trimmed, prefix) {
		return 0, false
	}
	rest := strings.TrimSpace(trimmed[len(prefix):])
	if nl := strings.IndexAny(rest, "\r\n"); nl >= 0 {
		rest = rest[:nl]
	}
	rest = strings.TrimSpace(rest)
	code, err := strconv.Atoi(rest)
	if err != nil {
		return 0, false
	}
	return code, true
}

func normalizedSectionBody(text, name string) string {
	marker := name + ":"
	idx := strings.Index(text, "\n"+marker)
	if idx < 0 {
		trimmed := strings.TrimSpace(text)
		if !strings.HasPrefix(trimmed, marker) {
			return ""
		}
		idx = strings.Index(text, marker) - 1
	}

	body := text[idx+1+len(marker):]
	if end := strings.Index(body, "\n\n"); end >= 0 {
		body = body[:end]
	}
	return strings.TrimSpace(body)
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func lastNonEmptyLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if trimmed := strings.TrimSpace(lines[i]); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func nonEmptyLineCount(s string) int {
	count := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func truncateSummaryLine(s string) string {
	const maxSummaryWidth = 72
	if len(s) <= maxSummaryWidth {
		return s
	}
	return s[:maxSummaryWidth-3] + "..."
}

func normalizeShellText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimSpace(s)
}

func stringFromMap(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := raw[key].(string); ok {
			return value
		}
	}
	return ""
}

func intFromMap(raw map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		switch value := raw[key].(type) {
		case float64:
			return int(value), true
		case int:
			return value, true
		}
	}
	return 0, false
}

func boolFromMap(raw map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		if value, ok := raw[key].(bool); ok {
			return value, true
		}
	}
	return false, false
}

func firstNonEmptyValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func resolveTargetAlias(args map[string]any) string {
	server := "local"
	if s, ok := args["server"].(string); ok && strings.TrimSpace(s) != "" {
		server = strings.TrimSpace(s)
	} else if active, ok := args["_active_workspace"].(string); ok && strings.TrimSpace(active) != "" {
		server = strings.TrimSpace(active)
	}

	label := server
	if asUser, ok := args["as_user"].(string); ok && strings.TrimSpace(asUser) != "" {
		label += "/" + strings.TrimSpace(asUser)
		return label
	}
	if channel, ok := args["channel"].(string); ok && strings.TrimSpace(channel) != "" {
		label += "/" + strings.TrimSpace(channel)
	}
	if role, ok := args["role"].(string); ok && strings.TrimSpace(role) != "" {
		label += " role=" + strings.TrimSpace(role)
	}
	return label
}
