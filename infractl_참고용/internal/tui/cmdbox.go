// Package tui
// File: cmdbox.go
// Description: 에이전트 도구 실행 결과를 박스 형태로 렌더링하는 컴포넌트
// Responsibility: cmdBox 타입 관리, 실행 상태별 렌더링, 결과 포맷팅

package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/yourorg/infractl/internal/diff"
)

type boxStatus int

const (
	boxRunning boxStatus = iota
	boxCompleted
	boxFailed
	boxGuard
	boxCancelled
)

const maxLiveOutputLines = 100

type cmdBox struct {
	toolName     string
	target       string
	args         map[string]interface{}
	result       string
	metadataJSON string
	rendered     string
	duration     time.Duration
	status       boxStatus
	collapse     bool
	sp           spinner.Model
	liveOutput   []string
	startedAt    time.Time
}

func newCmdBox(toolName, target string, args map[string]interface{}) cmdBox {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = StyleSpinner
	return cmdBox{
		toolName:  toolName,
		target:    target,
		args:      args,
		status:    boxRunning,
		sp:        s,
		startedAt: time.Now(),
	}
}

func (b *cmdBox) Init() tea.Cmd { return b.sp.Tick }

func (b *cmdBox) Update(msg tea.Msg) tea.Cmd {
	if b.status == boxRunning {
		var cmd tea.Cmd
		b.sp, cmd = b.sp.Update(msg)
		return cmd
	}
	return nil
}

func (b *cmdBox) SetCompleted(result string, dur time.Duration, success bool, metadataJSON string) {
	b.result = result
	b.metadataJSON = metadataJSON
	b.duration = dur
	if success {
		b.status = boxCompleted
	} else {
		b.status = boxFailed
	}
	b.rendered = b.renderResult(result)
}

func (b *cmdBox) SetGuardViolation(result string, dur time.Duration) {
	b.result = result
	b.duration = dur
	b.status = boxGuard
	b.rendered = b.renderResult(result)
}

func (b *cmdBox) AppendOutput(line string) {
	b.liveOutput = append(b.liveOutput, line)
	if len(b.liveOutput) > maxLiveOutputLines {
		b.liveOutput = b.liveOutput[len(b.liveOutput)-maxLiveOutputLines:]
	}
}

func (b *cmdBox) ToggleCollapse() { b.collapse = !b.collapse }

func (b *cmdBox) header() string {
	bullet := StyleClaude().Render("*")
	if b.status == boxGuard {
		bullet = StyleWarning.Render("!")
	} else if b.status == boxCancelled {
		bullet = StyleInfoBarDim.Render("—")
	}
	name := StyleCmdBoxToolName.Render(toolDisplayName(b.toolName))
	arg := toolDisplayArg(b.toolName, b.args)

	var parts []string
	if arg != "" {
		parts = []string{bullet + " " + name + StyleCmdBoxDim.Render("(") + arg + StyleCmdBoxDim.Render(")")}
	} else {
		parts = []string{bullet + " " + name}
	}

	if contextLabel := toolTargetLabel(b.toolName, b.target); contextLabel != "" {
		parts = append(parts, StyleCmdBoxDim.Render("·"), StyleCmdBoxTarget.Render(contextLabel))
	}

	switch b.status {
	case boxRunning:
		parts = append(parts, StyleCmdBoxDim.Render("·"), b.sp.View())
	case boxCompleted:
		dur := StyleCmdBoxDim.Render(fmt.Sprintf("(%.1fs)", b.duration.Seconds()))
		parts = append(parts, StyleCmdBoxDim.Render("·"), dur, StyleSuccess.Render("done"))
	case boxFailed:
		dur := StyleCmdBoxDim.Render(fmt.Sprintf("(%.1fs)", b.duration.Seconds()))
		parts = append(parts, StyleCmdBoxDim.Render("·"), dur, StyleError.Render("failed"))
	case boxGuard:
		dur := StyleCmdBoxDim.Render(fmt.Sprintf("(%.1fs)", b.duration.Seconds()))
		parts = append(parts, StyleCmdBoxDim.Render("·"), dur, StyleWarning.Render("guard"))
	case boxCancelled:
		dur := StyleCmdBoxDim.Render(fmt.Sprintf("(%.1fs)", b.duration.Seconds()))
		label := lipgloss.NewStyle().Foreground(ColorPlanRequest).Render("cancelled")
		parts = append(parts, StyleCmdBoxDim.Render("·"), dur, label)
	}

	return strings.Join(parts, " ")
}

func (b *cmdBox) View(width int) string {
	innerW := width - 6
	if innerW < 10 {
		innerW = 10
	}

	hdr := b.header()

	if b.status == boxRunning {
		var bodyParts []string
		bodyParts = append(bodyParts, StyleCmdBoxDim.Render(previewCmd(b.toolName, b.args)))

		if len(b.liveOutput) > 0 {
			const tailLines = 5
			total := len(b.liveOutput)
			start := total - tailLines
			if start < 0 {
				start = 0
			}
			for _, line := range b.liveOutput[start:] {
				bodyParts = append(bodyParts, StyleCmdBoxDim.Render(line))
			}
			if total > tailLines {
				hint := fmt.Sprintf("+%d lines", total-tailLines)
				bodyParts = append(bodyParts, StyleCmdBoxDim.Render(hint))
			}
		}

		elapsed := time.Since(b.startedAt).Truncate(100 * time.Millisecond)
		bodyParts = append(bodyParts, StyleCmdBoxDim.Render("elapsed "+formatElapsedShort(elapsed)))

		inner := hdr + "\n" + strings.Join(bodyParts, "\n")
		return StyleCmdBoxBorder.Width(innerW).Render(inner)
	}

	summary := toolSummary(b.toolName, b.args, b.result, b.metadataJSON, b.duration, b.status == boxCompleted)

	if b.collapse {
		line := StyleCmdBoxDim.Render(">") + " " + hdr + "\n" + summary
		return lipgloss.NewStyle().PaddingLeft(1).Render(line)
	}

	body := truncateResult(b.rendered)
	inner := hdr + "\n" + summary + "\n" + body

	borderStyle := StyleCmdBoxBorder
	if b.status == boxGuard {
		borderStyle = StyleCmdBoxGuardBorder
	} else if b.status == boxCancelled {
		borderStyle = StyleCmdBoxCancelBorder
	}
	return borderStyle.Width(innerW).Render(inner)
}

func (b *cmdBox) renderResult(result string) string {
	switch b.toolName {
	case "shell_exec":
		cmd, _ := b.args["command"].(string)
		return renderShellResult(cmd, result)
	case "file_write":
		if diff.IsDiff(result) {
			return RenderDiff(result, 0)
		}
		if idx := strings.Index(result, "\n\n"); idx != -1 {
			header := result[:idx]
			body := result[idx+2:]
			if diff.IsDiff(body) {
				return header + "\n\n" + RenderDiff(body, 0)
			}
		}
		return result
	case "file_read":
		return renderFileReadResult(result)
	default:
		return result
	}
}
