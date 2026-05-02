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

var ansiRegex = regexp.MustCompile("[][[()#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-7,A-ORZcf-nqry=><]")

const maxShellOutputBufferChars = 128 * 1024

func stripANSI(str string) string {
	return ansiRegex.ReplaceAllString(str, "")
}

func init() {
	toolui.Register("bash", &BashRenderer{})
	toolui.Register("powershell", &BashRenderer{})
}

// BashInteractiveModel implements toolui.InteractiveModel for shell tools.
type BashInteractiveModel struct {
	toolName     string
	targetAlias  string
	showTarget   bool
	isBackground bool
	command      string
	description  string
	theme        toolui.ThemeContext
	spinner      spinner.Model
	output       string
	isFocused    bool
	isExpanded   bool
	isFinished   bool
	result       string
	inputChan    chan string
}

func (m *BashInteractiveModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m *BashInteractiveModel) Update(msg tea.Msg) (toolui.InteractiveModel, tea.Cmd) {
	switch v := msg.(type) {
	case tea.KeyMsg:
		if m.isFocused && m.inputChan != nil && !m.isFinished {
			// 포커스 상태: 키를 프로세스 stdin으로 포워딩
			s := v.String()
			switch s {
			case "enter":
				s = "\n"
			case "ctrl+d":
				s = "\x04" // EOF — sqlplus/python 등 정상 종료 신호
			case "ctrl+c":
				s = "\x03" // SIGINT — 프로세스 중단
			default:
				if len(s) != 1 {
					return m, nil // 특수키 무시
				}
			}
			select {
			case m.inputChan <- s:
			default:
			}
			return m, nil
		}
		// 비포커스 상태: Ctrl+D → background 전환 신호
		if !m.isFocused && m.inputChan != nil && !m.isFinished && !m.isBackground {
			if v.String() == "ctrl+d" {
				select {
				case m.inputChan <- shellsignal.BackgroundRequest:
				default:
				}
				return m, nil
			}
		}
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

	lines := m.getFilteredLines()
	maxLines := 3
	if m.isExpanded {
		if m.isFinished {
			maxLines = len(lines)
		} else {
			maxLines = 40
		}
	}

	bodyLines := make([]string, 0, maxLines+4)
	switch {
	case len(lines) == 0:
		if m.isFinished {
			if m.isBackground {
				bodyLines = append(bodyLines, "Running in the background")
			} else {
				bodyLines = append(bodyLines, m.statusForFinishedNoOutput())
			}
		} else {
			bodyLines = append(bodyLines, "Running...")
		}
	case len(lines) > maxLines:
		hidden := len(lines) - maxLines
		toggleHint := "Ctrl+O로 펼치기"
		if m.isFinished {
			// 완료 후 스크롤백으로 넘어간 뒤에는 더 이상 상호작용이 불가능하므로 힌트 변경
			toggleHint = "생략됨"
		} else if m.isExpanded {
			toggleHint = "Ctrl+O로 접기"
		}
		bodyLines = append(bodyLines, lines[:maxLines]...)
		bodyLines = append(bodyLines, fmt.Sprintf("... (%d줄 숨김, %s) ...", hidden, toggleHint))
	default:
		bodyLines = append(bodyLines, lines...)
	}

	body := toolui.FormatResultLines(bodyLines, true, false, m.theme)

	var hint string
	switch {
	case m.isFinished:
		hint = ""
	case m.isBackground:
		hint = toolui.FormatHintLine("(tab to focus)", m.isFocused, m.theme)
	case m.isFocused:
		hint = toolui.FormatHintLine("(tab/esc to defocus) · (ctrl+d for EOF) · (ctrl+c for INT)", true, m.theme)
	default:
		hint = toolui.FormatHintLine("(tab to focus) · (ctrl+d to background)", false, m.theme)
	}

	out := headline + "\n" + body
	if hint != "" {
		out += "\n" + hint
	}
	return out
}

// statusForFinishedNoOutput는 stdout/stderr가 모두 비어 종료한 명령에 대해
// `Done` 또는 `(No output)` 마무리 라인을 반환한다.
// claude_cli와 동일하게 mv/rm/touch/mkdir 등 무출력 정상 명령은 `Done`,
// 그 외 명령은 `(No output)`.
func (m *BashInteractiveModel) statusForFinishedNoOutput() string {
	cmd := strings.TrimSpace(m.command)
	if cmd == "" {
		return "(No output)"
	}
	first := strings.ToLower(strings.Fields(cmd)[0])
	switch first {
	case "mv", "rm", "touch", "mkdir", "cp", "chmod", "chown", "ln", "rmdir":
		return "Done"
	}
	return "(No output)"
}

func (m *BashInteractiveModel) getFilteredLines() []string {
	cleanOutput := strings.ReplaceAll(m.output, "\r\n", "\n")
	cleanOutput = stripANSI(cleanOutput)

	allLines := strings.Split(cleanOutput, "\n")
	var filtered []string
	for _, line := range allLines {
		trimmed := strings.TrimSpace(line)
		// 내부 보안 토큰은 필터링
		if strings.Contains(trimmed, "__ARG_M_") ||
			strings.Contains(trimmed, "ARGUS_SU_PW") {
			continue
		}
		filtered = append(filtered, line)
	}
	// PowerShell/bash 출력은 종종 빈 줄로 시작·끝나는데, 그대로 두면
	// `  ⎿` 옆이 비어 보이거나 마지막에 흰 공간이 남는다. trim해 표시 정렬을 맞춘다.
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
func (m *BashInteractiveModel) SetResult(_ string)                 {}

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
	targetAlias := resolveTargetAlias(args)
	background, _ := args["background"].(bool)

	showTarget := true

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = theme.Style(theme.StatusWarningColor())

	return &BashInteractiveModel{
		toolName:     toolName,
		targetAlias:  targetAlias,
		showTarget:   showTarget,
		isBackground: background,
		command:      command,
		description:  description,
		theme:        theme,
		spinner:      s,
	}
}

func (r *BashRenderer) RenderToolUse(args map[string]any, streamBody string, theme toolui.ThemeContext) string {
	command, _ := args["command"].(string)
	command = collapseToSingleLine(command)
	toolName, _ := args["_tool_name"].(string)
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		toolName = "bash"
	}
	targetAlias := resolveTargetAlias(args)
	background, _ := args["background"].(bool)

	showTarget := true

	s := &BashInteractiveModel{
		toolName:     toolName,
		targetAlias:  targetAlias,
		showTarget:   showTarget,
		isBackground: background,
		command:      command,
		theme:        theme,
		output:       streamBody,
	}
	return s.View()
}

func (r *BashRenderer) RenderToolResult(resultText string, durationMs int64, theme toolui.ThemeContext) string {
	var execResult struct {
		Stdout string `json:"Stdout"`
		Stderr string `json:"Stderr"`
		Code   int    `json:"Code"`
	}

	// 성공: InteractiveModel.View()가 이미 stdout/Done을 표시하므로 추가 출력 없음.
	if err := json.Unmarshal([]byte(resultText), &execResult); err == nil {
		if execResult.Code == 0 {
			return ""
		}

		stderrLine := firstNonEmptyLine(stripANSI(execResult.Stderr))

		// exit 1 (보통 grep 결과 없음) + stderr 없음 = InteractiveModel이 이미 (No output) 등을 처리함.
		if execResult.Code == 1 && stderrLine == "" {
			return ""
		}

		// 비정상 종료: exit code + stderr 첫 줄을 적색 라인으로 추가.
		label := fmt.Sprintf("exit %d", execResult.Code)
		if durationMs > 0 {
			label += fmt.Sprintf(" (%dms)", durationMs)
		}
		lines := []string{label}
		if stderrLine != "" {
			lines = []string{stderrLine, label}
		}
		return toolui.FormatResultLines(lines, true, true, theme)
	}

	// distiller 정규화 텍스트 (`exit_code: <N>\n\nstdout:...`) 처리.
	if code, ok := parseNormalizedExitCode(resultText); ok {
		if code == 0 {
			return ""
		}

		stderrLine := firstNormalizedSection(resultText, "stderr")

		if code == 1 && stderrLine == "" {
			return ""
		}

		label := fmt.Sprintf("exit %d", code)
		if durationMs > 0 {
			label += fmt.Sprintf(" (%dms)", durationMs)
		}
		lines := []string{label}
		if stderrLine != "" {
			lines = []string{stderrLine, label}
		}
		return toolui.FormatResultLines(lines, true, true, theme)
	}

	// JSON 파싱 실패: tool 자체가 NewErrorEvent를 발행한 케이스.
	errMsg := firstNonEmptyLine(strings.TrimSpace(resultText))
	if errMsg == "" || strings.HasPrefix(errMsg, "{") {
		errMsg = "execution failed"
	}
	if durationMs > 0 {
		errMsg += fmt.Sprintf(" (%dms)", durationMs)
	}
	return toolui.FormatResultLines([]string{errMsg}, true, true, theme)
}

// parseNormalizedExitCode 는 "exit_code: <N>" 형태로 시작하는 distiller 정규화 텍스트에서
// 종료 코드를 추출한다. 매칭되지 않으면 ok=false.
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

// firstNormalizedSection 은 distiller 정규화 텍스트에서 "<name>:\n..." 섹션의 첫 비-공백 줄을 반환한다.
func firstNormalizedSection(text, name string) string {
	marker := name + ":"
	idx := strings.Index(text, "\n"+marker)
	if idx < 0 {
		if strings.HasPrefix(strings.TrimSpace(text), marker) {
			idx = strings.Index(text, marker) - 1
		} else {
			return ""
		}
	}
	body := text[idx+1+len(marker):]
	return firstNonEmptyLine(stripANSI(body))
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
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
	if channel, ok := args["channel"].(string); ok && strings.TrimSpace(channel) != "" {
		label += "/" + strings.TrimSpace(channel)
	}
	if role, ok := args["role"].(string); ok && strings.TrimSpace(role) != "" {
		label += " role=" + strings.TrimSpace(role)
	}
	return label
}
