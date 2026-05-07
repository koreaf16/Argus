package servercopy

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/koreaf16/argus/internal/tui/toolui"
)

func init() {
	toolui.Register("server_copy", &ServerCopyRenderer{})
}

type ServerCopyInteractiveModel struct {
	src        string
	dst        string
	theme      toolui.ThemeContext
	spinner    spinner.Model
	progress   progress.Model
	current    int64
	total      int64
	percent    float64
	isFinished bool
	err        error
}

func (m *ServerCopyInteractiveModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m *ServerCopyInteractiveModel) Update(msg tea.Msg) (toolui.InteractiveModel, tea.Cmd) {
	switch v := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(v)
		return m, cmd
	case progress.FrameMsg:
		var cmd tea.Cmd
		updated, cmd := m.progress.Update(v)
		m.progress = updated.(progress.Model)
		return m, cmd
	}
	return m, nil
}

func (m *ServerCopyInteractiveModel) View() string {
	headline := toolui.FormatToolCall("ServerCopy", fmt.Sprintf("%s -> %s", m.src, m.dst), 160, m.theme)

	var status string
	if m.isFinished {
		if m.err != nil {
			status = m.theme.Style(m.theme.StatusErrorColor()).Render(fmt.Sprintf("✘ Failed: %v", m.err))
		} else {
			status = m.theme.Style(m.theme.StatusSuccessColor()).Render("✔ Successfully copied")
		}
	} else {
		status = fmt.Sprintf("%s Transferring... %s", m.spinner.View(), formatBytes(m.current))
		if m.total > 0 {
			status += fmt.Sprintf(" / %s", formatBytes(m.total))
		}
	}

	progressBar := m.progress.ViewAs(m.percent / 100.0)

	bodyLines := []string{
		status,
		progressBar,
	}

	body := toolui.FormatResultLines(bodyLines, true, false, m.theme)
	return headline + "\n" + body
}

func (m *ServerCopyInteractiveModel) OnStreamDelta(delta string) {
	if strings.HasPrefix(delta, "PROGRESS:") {
		parts := strings.Split(delta, ":")
		if len(parts) >= 4 {
			m.current, _ = strconv.ParseInt(parts[1], 10, 64)
			m.total, _ = strconv.ParseInt(parts[2], 10, 64)
			m.percent, _ = strconv.ParseFloat(parts[3], 64)
		}
	}
}

func (m *ServerCopyInteractiveModel) SetFinished(finished bool) { m.isFinished = finished }
func (m *ServerCopyInteractiveModel) SetResult(result string) {
	trimmed := strings.TrimSpace(result)
	if trimmed == "" {
		return
	}
	if strings.Contains(trimmed, "Successfully") ||
		strings.Contains(trimmed, "성공적으로 복사했습니다") ||
		strings.Contains(trimmed, "copied ") {
		m.err = nil
		return
	}
	m.err = fmt.Errorf("%s", trimmed)
}
func (m *ServerCopyInteractiveModel) SetFocus(focus bool)          {}
func (m *ServerCopyInteractiveModel) IsFocused() bool              { return false }
func (m *ServerCopyInteractiveModel) SetExpanded(exp bool)         {}
func (m *ServerCopyInteractiveModel) IsExpanded() bool             { return false }
func (m *ServerCopyInteractiveModel) SetInputResponse(chan string) {}

type ServerCopyRenderer struct{}

func (r *ServerCopyRenderer) CreateInteractiveModel(args map[string]any, theme toolui.ThemeContext) toolui.InteractiveModel {
	src, _ := args["src"].(string)
	srcServer, _ := args["src_server"].(string)
	srcRole, _ := args["src_role"].(string)
	if src == "" {
		src, _ = args["src_path"].(string)
		if src != "" && strings.TrimSpace(srcServer) != "" {
			src = strings.TrimSpace(srcServer) + ":" + src
		}
	}
	if strings.TrimSpace(srcRole) != "" {
		src = strings.TrimSpace(srcRole) + "@" + src
	}
	dst, _ := args["dst"].(string)
	dstServer, _ := args["dst_server"].(string)
	dstRole, _ := args["dst_role"].(string)
	if dst == "" {
		dst, _ = args["dst_path"].(string)
		if dst != "" && strings.TrimSpace(dstServer) != "" {
			dst = strings.TrimSpace(dstServer) + ":" + dst
		}
	}
	if strings.TrimSpace(dstRole) != "" {
		dst = strings.TrimSpace(dstRole) + "@" + dst
	}

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = theme.Style(theme.StatusWarningColor())

	p := progress.New(progress.WithDefaultGradient())
	p.Width = 40

	return &ServerCopyInteractiveModel{
		src:      src,
		dst:      dst,
		theme:    theme,
		spinner:  s,
		progress: p,
	}
}

func (r *ServerCopyRenderer) RenderToolUse(args map[string]any, delta string, theme toolui.ThemeContext) string {
	m := r.CreateInteractiveModel(args, theme).(*ServerCopyInteractiveModel)
	m.OnStreamDelta(delta)
	return m.View()
}

func (r *ServerCopyRenderer) RenderToolResult(result string, _ int64, theme toolui.ThemeContext) string {
	// Successful result is already handled by InteractiveModel's final view.
	if strings.Contains(result, "Successfully") {
		return ""
	}
	// Return error result in red if it looks like an error.
	return toolui.FormatResultLines([]string{result}, true, true, theme)
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("% d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
