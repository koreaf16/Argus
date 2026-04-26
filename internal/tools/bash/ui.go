package bash

import (
	"encoding/json"
	"fmt"
	"regexp"
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
		if m.inputChan != nil && !m.isFinished {
			s := v.String()
			if s == "ctrl+d" && !m.isBackground {
				select {
				case m.inputChan <- shellsignal.BackgroundRequest:
				default:
				}
				return m, nil
			}
		}
		if m.isFocused && m.inputChan != nil && !m.isFinished {
			s := v.String()
			if s == "enter" {
				s = "\n"
			} else if len(s) == 1 {
				// OK
			} else {
				return m, nil
			}
			// Non-blocking send or separate goroutine recommended in production
			select {
			case m.inputChan <- s:
			default:
			}
			return m, nil
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(v)
		return m, cmd
	}
	return m, nil
}

func (m *BashInteractiveModel) View() string {
	boxW := m.theme.Width() - 4 // ?몃뜶??怨좊젮
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
		displayLine := truncateToWidth(headerText, boxW-2)
		return "  " + displayLine
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
			sb.WriteString(mutedStyle.Render(fmt.Sprintf("   ... %d lines hidden (Ctrl+O to show) ...\n", hidden)))
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
		footer := "   [TAB to defocus] [PTY Input Active]"
		if !m.isBackground && !m.isFinished {
			footer = "   [TAB to defocus] [Ctrl+D background] [PTY Input Active]"
		}
		sb.WriteString(m.theme.Style(m.theme.StatusWarningColor()).Bold(true).Render(footer))
	} else if !m.isFinished {
		sb.WriteString("\n")
		footer := "   [TAB to focus]"
		if !m.isBackground {
			footer = "   [TAB to focus] [Ctrl+D background]"
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
	
	// 성공 시 RenderToolUse에서 이미 ✓ 또는 박스를 표시하므로, 
	// 추가 결과 텍스트를 숨겨서 UI를 깔끔하게 유지 (gemini-cli 방식)
	if err := json.Unmarshal([]byte(resultText), &execResult); err == nil && execResult.Code == 0 {
		return ""
	} else if err == nil && execResult.Code != 0 {
		label := fmt.Sprintf("✗ [error] exit code %d", execResult.Code)
		color := theme.StatusErrorColor()
		msg := label
		if durationMs > 0 {
			msg += fmt.Sprintf(" in %dms", durationMs)
		}
		return theme.Style(color).Render("  " + msg)
	}

	// JSON 파싱 실패 = 도구가 에러 이벤트를 반환한 경우
	errMsg := strings.TrimSpace(resultText)
	if errMsg == "" {
		errMsg = "execution failed"
	}
	label := "✗ [error] " + errMsg
	if durationMs > 0 {
		label += fmt.Sprintf(" in %dms", durationMs)
	}
	return theme.Style(theme.StatusErrorColor()).Render("  " + label)
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
