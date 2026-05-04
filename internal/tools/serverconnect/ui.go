package serverconnect

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/koreaf16/argus/internal/tui/toolui"
)

func init() {
	toolui.Register("server_connect", &ServerConnectRenderer{})
}

// branchPrefix / contPrefix는 toolui.inline.go의 unexported 상수와 동일한 값.
const (
	scBranchPrefix = "  ⎿  "
	scContPrefix   = "     "
)

type serverConnectPhase int

const (
	phaseConnecting serverConnectPhase = iota
	phaseInspecting
)

type ServerConnectInteractiveModel struct {
	alias      string
	theme      toolui.ThemeContext
	sp         spinner.Model
	phase      serverConnectPhase
	details    string
	errResult  string
	isFinished bool
}

func (m *ServerConnectInteractiveModel) Init() tea.Cmd { return m.sp.Tick }

func (m *ServerConnectInteractiveModel) Update(msg tea.Msg) (toolui.InteractiveModel, tea.Cmd) {
	if tick, ok := msg.(spinner.TickMsg); ok {
		var cmd tea.Cmd
		m.sp, cmd = m.sp.Update(tick)
		return m, cmd
	}
	return m, nil
}

func (m *ServerConnectInteractiveModel) View() string {
	headline := toolui.FormatToolCall("ServerConnect", m.alias, 80, m.theme)
	branch := m.theme.Style(m.theme.MutedColor()).Render(scBranchPrefix)

	var body strings.Builder
	if m.isFinished {
		if m.errResult != "" {
			errStyle := m.theme.Style(m.theme.StatusErrorColor())
			body.WriteString(branch)
			body.WriteString(errStyle.Render("✘ " + m.errResult))
		} else {
			successStyle := m.theme.Style(m.theme.StatusSuccessColor())
			body.WriteString(branch)
			body.WriteString(successStyle.Render("✔ Connected to " + m.alias))
			if m.details != "" {
				body.WriteString("\n")
				body.WriteString(scContPrefix)
				body.WriteString(m.theme.Style(m.theme.MutedColor()).Render(m.details))
			}
		}
	} else {
		var phaseText string
		switch m.phase {
		case phaseInspecting:
			phaseText = fmt.Sprintf("%s Inspecting...", m.sp.View())
		default:
			phaseText = fmt.Sprintf("%s Connecting to %s...", m.sp.View(), m.alias)
		}
		body.WriteString(branch)
		body.WriteString(phaseText)
	}

	return headline + "\n" + body.String()
}

func (m *ServerConnectInteractiveModel) OnStreamDelta(delta string) {
	if strings.Contains(delta, "Connected to") {
		m.phase = phaseInspecting
		return
	}
	if m.phase == phaseInspecting {
		if d := strings.TrimSpace(delta); d != "" {
			m.details = d
		}
	}
}

func (m *ServerConnectInteractiveModel) SetFinished(finished bool) { m.isFinished = finished }

func (m *ServerConnectInteractiveModel) SetResult(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	// 에러 판별: "failed" 또는 "error" 접두사
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "failed") || strings.HasPrefix(lower, "error") {
		m.errResult = text
		return
	}
	// inspect 요약 라인 추출 (아직 details가 없을 때 폴백)
	if m.details == "" {
		for _, line := range strings.Split(text, "\n") {
			l := strings.TrimSpace(line)
			if strings.Contains(l, "OS:") || strings.Contains(l, "user:") || strings.Contains(l, "shell:") {
				m.details = l
				break
			}
		}
	}
}

func (m *ServerConnectInteractiveModel) SetFocus(bool)            {}
func (m *ServerConnectInteractiveModel) IsFocused() bool          { return false }
func (m *ServerConnectInteractiveModel) SetExpanded(bool)         {}
func (m *ServerConnectInteractiveModel) IsExpanded() bool         { return false }
func (m *ServerConnectInteractiveModel) SetInputResponse(chan string) {}

// ── Renderer ─────────────────────────────────────────────────────────────────

type ServerConnectRenderer struct{}

func (r *ServerConnectRenderer) CreateInteractiveModel(args map[string]any, theme toolui.ThemeContext) toolui.InteractiveModel {
	alias, _ := args["server"].(string)
	alias = strings.TrimSpace(alias)
	if alias == "" {
		alias = "?"
	}
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = theme.Style(theme.ClaudeAccentColor())
	return &ServerConnectInteractiveModel{alias: alias, theme: theme, sp: s}
}
func (r *ServerConnectRenderer) RenderToolUse(args map[string]any, streamBody string, theme toolui.ThemeContext) string {
	alias, _ := args["server"].(string)
	alias = strings.TrimSpace(alias)
	if alias == "" {
		alias = "?"
	}
	headline := toolui.FormatToolCall("ServerConnect", alias, 80, theme)

	streamBody = strings.TrimSpace(streamBody)
	if streamBody == "" {
		return headline
	}

	var lines []string
	if strings.Contains(streamBody, "Connected to") {
		lines = append(lines, "✔ "+alias+"에 연결됨")
		for _, line := range strings.Split(streamBody, "\n") {
			l := strings.TrimSpace(line)
			if strings.Contains(l, "OS:") || strings.Contains(l, "user:") {
				lines = append(lines, l)
				break
			}
		}
	} else {
		for _, line := range strings.Split(streamBody, "\n") {
			if l := strings.TrimSpace(line); l != "" {
				lines = append(lines, l)
			}
		}
	}
	if len(lines) == 0 {
		return headline
	}
	return headline + "\n" + toolui.FormatResultLines(lines, true, false, theme)
}

func (r *ServerConnectRenderer) RenderToolResult(resultText string, _ int64, theme toolui.ThemeContext) string {
	resultText = strings.TrimSpace(resultText)
	if resultText == "" {
		return ""
	}
	// 성공 결과는 InteractiveModel이 이미 표시하므로 생략.
	if strings.Contains(resultText, "Connected to") {
		return ""
	}
	// 에러 결과
	var lines []string
	for _, line := range strings.Split(resultText, "\n") {
		if l := strings.TrimSpace(line); l != "" {
			lines = append(lines, l)
		}
	}
	return toolui.FormatResultLines(lines, true, true, theme)
}
