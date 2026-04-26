package askuser

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tui/toolui"
)

func init() {
	toolui.Register("ask_user", &AskUserRenderer{})
}

type AskUserInteractiveModel struct {
	questions  []tool.AskUserQuestion
	cursor     int
	answers    map[string]string
	response   chan tool.AskUserBatchResponse
	isFocused  bool
	isFinished bool
	theme      toolui.ThemeContext
}

func (m *AskUserInteractiveModel) Init() tea.Cmd { return nil }

func (m *AskUserInteractiveModel) Update(msg tea.Msg) (toolui.InteractiveModel, tea.Cmd) {
	if m.isFinished || !m.isFocused {
		return m, nil
	}

	switch v := msg.(type) {
	case tea.KeyMsg:
		switch v.String() {
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down":
			if m.cursor < len(m.questions[0].Options)-1 {
				m.cursor++
			}
		case "enter":
			// 일단 첫 번째 질문의 선택지만 처리 (데모용)
			if len(m.questions) > 0 && len(m.questions[0].Options) > 0 {
				ans := m.questions[0].Options[m.cursor].Value
				m.answers["0"] = ans
				m.response <- tool.AskUserBatchResponse{
					AnswersByIndex: m.answers,
				}
				m.isFinished = true
			}
		}
	}
	return m, nil
}

func (m *AskUserInteractiveModel) View() string {
	boxW := max(m.theme.Width()-4, 30)
	if len(m.questions) == 0 {
		return ""
	}

	q := m.questions[0]
	var sb strings.Builder
	sb.WriteString(m.theme.Style(m.theme.ToolUseColor()).Bold(true).Render("  ? " + q.Question + "\n\n"))

	for i, opt := range q.Options {
		cursor := "  "
		style := m.theme.Style(m.theme.BodyColor())
		if i == m.cursor && m.isFocused && !m.isFinished {
			cursor = "> "
			style = m.theme.Style(m.theme.StatusWarningColor()).Bold(true)
		}
		sb.WriteString(style.Render(fmt.Sprintf("    %s %s\n", cursor, opt.Label)))
	}

	if m.isFinished {
		sb.WriteString("\n" + m.theme.Style(m.theme.StatusSuccessColor()).Render("    ✓ Answered"))
	} else if m.isFocused {
		sb.WriteString("\n" + m.theme.Style(m.theme.MutedColor()).Render("    [UP/DOWN to select, ENTER to confirm]"))
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.theme.BorderColor())).
		Width(boxW).
		Render(sb.String())
}

func (m *AskUserInteractiveModel) SetFocus(focus bool)                { m.isFocused = focus }
func (m *AskUserInteractiveModel) IsFocused() bool                    { return m.isFocused }
func (m *AskUserInteractiveModel) SetExpanded(expanded bool)          {}
func (m *AskUserInteractiveModel) IsExpanded() bool                   { return false }
func (m *AskUserInteractiveModel) OnStreamDelta(delta string)         {}
func (m *AskUserInteractiveModel) SetInputResponse(input chan string) {}
func (m *AskUserInteractiveModel) SetFinished(finished bool)          { m.isFinished = finished }

type AskUserRenderer struct{}

func (r *AskUserRenderer) CreateInteractiveModel(args map[string]any, theme toolui.ThemeContext) toolui.InteractiveModel {
	// 실제 환경에서는 EventAskUserRequest를 통해 전달된 Questions와 Response 채널을 써야 함.
	return nil
}

func (r *AskUserRenderer) RenderToolUse(args map[string]any, _ string, theme toolui.ThemeContext) string {
	question, _ := args["question"].(string)
	return theme.Style(theme.ToolUseColor()).Bold(true).Render("  Ask: ") + theme.Style(theme.BodyColor()).Render(question)
}

func (r *AskUserRenderer) RenderToolResult(resultText string, durationMs int64, theme toolui.ThemeContext) string {
	var res map[string]any
	if err := json.Unmarshal([]byte(resultText), &res); err == nil {
		if canceled, _ := res["canceled"].(bool); canceled {
			return theme.Style(theme.StatusWarningColor()).Render("  [canceled]")
		}
	}
	return theme.Style(theme.StatusSuccessColor()).Render("  [answered]")
}
