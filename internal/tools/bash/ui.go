package bash

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/koreaf16/argus/internal/tools/shellsignal"
	"github.com/koreaf16/argus/internal/tui/toolui"
)

var ansiRegex = regexp.MustCompile("[\u001b\u009b][[()#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-7,A-ORZcf-nqry=><]")

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
				s = "\x04" // EOF — sqlplus/python 등 정상 종료
			case "ctrl+c":
				s = "\x03" // SIGINT — 프로세스 중단
			default:
				if len(s) != 1 {
					return m, nil // F1, 방향키 등 특수키 무시
				}
			}
			select {
			case m.inputChan <- s:
			default:
			}
			return m, nil
		}
		// 비포커스 상태: Ctrl+D → Kill 요청
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

func (m *BashInteractiveModel) View() string {
	// theme.Width()는 이미 아이콘 영역(3칸)을 제외한 가용 너비를 반환한다.
	// render.go가 박스 전체에 2칸 들여쓰기를 다시 추가하므로 그만큼만 더 빼준다.
	boxW := m.theme.Width() - 2
	if boxW < 40 {
		boxW = 40
	}

	mutedStyle := m.theme.Style(m.theme.MutedColor())
	bodyStyle := m.theme.Style(m.theme.BodyColor())

	// ?곹깭???곕Ⅸ ?뚮몢由??됱긽 寃곗젙
	borderColor := m.theme.BorderColor() // 湲곕낯
	if !m.isFinished {
		borderColor = m.theme.ToolUseColor() // ?ㅽ뻾 以?
	}

	// 상단 상태 아이콘
	var statusIcon string
	if m.isFinished {
		statusIcon = m.theme.Style(m.theme.StatusSuccessColor()).Render("✓")
	} else {
		spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		frame := spinnerFrames[(m.theme.AnimFrame()/2)%len(spinnerFrames)]
		statusIcon = m.theme.Style(m.theme.StatusWarningColor()).Render(frame)
	}

	headerText := fmt.Sprintf("%s %s", statusIcon, m.toolName)
	if m.showTarget {
		target := strings.TrimSpace(m.targetAlias)
		if target == "" {
			target = "local"
		}
		headerText += fmt.Sprintf(" [%s]", target)
	}
	headerText += " " + m.command

	lines := m.getFilteredLines()
	hasOutput := len(lines) > 0
	showBox := hasOutput || m.isExpanded || m.isFocused

	if !showBox {
		// 패딩과 아이콘은 render.go(renderTranscriptEntryAt)가 통일적으로 추가한다.
		return truncateToWidth(headerText, boxW-2)
	}

	// 박스 헤더 구성
	boxHeader := fmt.Sprintf("%s %s", statusIcon, m.toolName)
	if m.showTarget {
		boxHeader += fmt.Sprintf(" [%s]", m.targetAlias)
	}
	if !m.isFinished {
		boxHeader += mutedStyle.Render(" (ctrl+o to collapse)")
	}

	var sb strings.Builder
	sb.WriteString(boxHeader)
	sb.WriteString("\n\n")

	// 명령어 표시
	cmdDisplay := truncateToWidth(m.command, boxW-6)
	sb.WriteString(bodyStyle.Render("   $ " + cmdDisplay))

	if m.description != "" {
		sb.WriteString("\n")
		sb.WriteString(mutedStyle.Render("     " + truncateToWidth(m.description, boxW-6)))
	}

	if hasOutput {
		sb.WriteString("\n")
		maxLines := 8
		if m.isExpanded {
			maxLines = 40
		}

		if len(lines) > maxLines && !m.isExpanded {
			hidden := len(lines) - maxLines
			// 줄바꿈을 Render 밖으로 빼야 lipgloss가 다음 라인을 빈 스타일 잔재 없이 정상 렌더링한다.
			sb.WriteString(mutedStyle.Render(fmt.Sprintf("   ... %d lines hidden (Ctrl+O to show) ...", hidden)))
			sb.WriteString("\n")
			lines = lines[len(lines)-maxLines:]
		}

		for i, line := range lines {
			// ??諛??쒖뼱 臾몄옄 泥섎━
			line = strings.ReplaceAll(line, "\t", "    ")
			line = strings.ReplaceAll(line, "\r", "")

			displayLine := truncateToWidth(line, boxW-6)
			sb.WriteString(bodyStyle.Render("   " + displayLine))
			if i < len(lines)-1 {
				sb.WriteString("\n")
			}
		}
	}

	if m.isFocused {
		sb.WriteString("\n")
		footer := "   [TAB/ESC to defocus] [Ctrl+D=EOF] [Ctrl+C=INT]"
		sb.WriteString(m.theme.Style(m.theme.StatusWarningColor()).Bold(true).Render(footer))
	} else if !m.isFinished {
		sb.WriteString("\n")
		footer := "   [TAB to focus]"
		if !m.isBackground {
			footer = "   [TAB to focus] [Ctrl+D to kill]"
		}
		sb.WriteString(mutedStyle.Render(footer))
	}

	// 최종 스타일 적용
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		PaddingRight(1).
		Width(boxW).
		MaxWidth(boxW + 2).
		Render(sb.String())
	}

func truncateToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	runes := []rune(s)
	res := ""
	for _, r := range runes {
		if lipgloss.Width(res+string(r)) > width {
			break
		}
		res += string(r)
	}
	return res
}

func (m *BashInteractiveModel) getFilteredLines() []string {
	// \r\n -> \n 변환 및 ANSI 제거
	cleanOutput := strings.ReplaceAll(m.output, "\r\n", "\n")
	cleanOutput = stripANSI(cleanOutput)

	allLines := strings.Split(cleanOutput, "\n")
	var filtered []string
	for _, line := range allLines {
		trimmed := strings.TrimSpace(line)
		// 내부 보안 토큰 등은 필터링하되, 일반 실행 결과는 모두 유지
		if strings.Contains(trimmed, "__ARG_M_") ||
			strings.Contains(trimmed, "ARGUS_SU_PW") {
			continue
		}
		// 빈 라인이거나 기호만 있는 라인도 가독성을 위해 어느 정도 허용
		filtered = append(filtered, line)
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

	active, _ := args["_active_workspace"].(string)
	active = strings.TrimSpace(active)
	if active == "" {
		active = "local"
	}
	// ?ㅽ뻾 ??곸씠 ?꾩옱 ?쒖꽦 ?뚰겕?ㅽ럹?댁뒪? ?ㅻ? ?뚮쭔 ?쒖떆
	showTarget := targetAlias != active

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
	toolName, _ := args["_tool_name"].(string)
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		toolName = "bash"
	}
	targetAlias := resolveTargetAlias(args)
	background, _ := args["background"].(bool)

	active, _ := args["_active_workspace"].(string)
	active = strings.TrimSpace(active)
	if active == "" {
		active = "local"
	}
	showTarget := targetAlias != active

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

	// 성공: RenderToolUse(InteractiveModel)에서 이미 ✓ 박스를 표시하므로 추가 출력 없음 (gemini-cli 방식).
	if err := json.Unmarshal([]byte(resultText), &execResult); err == nil {
		if execResult.Code == 0 {
			return ""
		}
		// 비정상 종료: exit code + stderr 첫 줄만 짧게 표시
		errStyle := theme.Style(theme.StatusErrorColor())
		label := fmt.Sprintf("✗ exit %d", execResult.Code)
		if durationMs > 0 {
			label += fmt.Sprintf(" (%dms)", durationMs)
		}
		stderrLine := firstNonEmptyLine(stripANSI(execResult.Stderr))
		if stderrLine != "" {
			label += " — " + truncateToWidth(stderrLine, theme.Width()-len(label)-6)
		}
		return errStyle.Render("  " + label)
	}

	// 엔진 distiller가 JSON을 "exit_code: <N>\n\nstdout:..." 형태로 정규화한 경우를 처리.
	// 이 형식이 그대로 UI에 도달하면 fallback 경로가 "exit_code: 0"을 에러처럼 출력하는 문제가 있다.
	if code, ok := parseNormalizedExitCode(resultText); ok {
		if code == 0 {
			return ""
		}
		errStyle := theme.Style(theme.StatusErrorColor())
		label := fmt.Sprintf("✗ exit %d", code)
		if durationMs > 0 {
			label += fmt.Sprintf(" (%dms)", durationMs)
		}
		if stderrLine := firstNormalizedSection(resultText, "stderr"); stderrLine != "" {
			label += " — " + truncateToWidth(stderrLine, theme.Width()-len(label)-6)
		}
		return errStyle.Render("  " + label)
	}

	// JSON 파싱 실패: tool 자체가 NewErrorEvent를 발행한 케이스. resultText는
	// 사람이 읽을 수 있는 에러 문자열일 수도, raw JSON 잔해일 수도 있다.
	// 후자가 사용자 화면에 그대로 노출되지 않도록 한 줄로만 잘라 표시.
	errMsg := firstNonEmptyLine(strings.TrimSpace(resultText))
	if errMsg == "" || strings.HasPrefix(errMsg, "{") {
		errMsg = "execution failed"
	}
	label := "✗ " + truncateToWidth(errMsg, theme.Width()-6)
	if durationMs > 0 {
		label += fmt.Sprintf(" (%dms)", durationMs)
	}
	return theme.Style(theme.StatusErrorColor()).Render("  " + label)
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
	if server, ok := args["server"].(string); ok && strings.TrimSpace(server) != "" {
		return strings.TrimSpace(server)
	}
	if active, ok := args["_active_workspace"].(string); ok && strings.TrimSpace(active) != "" {
		return strings.TrimSpace(active)
	}
	return "local"
}
