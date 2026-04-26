package tui

import (
	"fmt"
	"strings"
	"time"
)

func (m AppModel) View() string {
	if m.width == 0 {
		return "initializing..."
	}

	if m.histOverlay.isActive() {
		overlay := m.histOverlay.View(&m.history, m.width)
		if m.parker != nil {
			m.parker.SetTarget(0, 0, false)
		}
		return "\033[?25l" + overlay + ansiClearToEnd
	}

	// 선택 UI 활성 시: 채팅·입력박스·푸터를 숨기고 선택 박스만 표시
	if m.selection.active {
		if m.parker != nil {
			m.parker.SetTarget(0, 0, false)
		}
		return "\033[?25l" + m.selection.View(m.width) + "\n" + m.statusBar.View() + ansiClearToEnd
	}

	var parts []string

	if m.busy && len(m.streamLines) > 0 {
		parts = append(parts, strings.Join(m.streamLines, "\n"))
	}
	// 실행 중 도구 박스 표시 (→ ToolName <arg> + streaming output)
	if m.busy {
		fgTools := m.activeTools.ForegroundTools()
		if len(fgTools) > 1 {
			elapsed := time.Duration(0)
			if len(fgTools) > 0 {
				elapsed = time.Since(fgTools[0].startTime)
			}
			parts = append(parts, renderParallelActivePanel(fgTools, elapsed, m.width))
		} else if st := m.activeTools.MostRecentForeground(); st != nil {
			elapsed := time.Since(st.startTime)
			parts = append(parts, renderShellBoxRunning(st.toolName, st.args, st.shellLines, st.shellTotal, elapsed, m.width))
		}
	}
	if m.queue.Len() > 0 {
		parts = append(parts, m.queue.View(m.width))
	}
	// 폼 입력 모드 활성 시: 폼 상태 박스 표시 (fake cursor 포함)
	if m.form.active {
		cursorOff := m.input.ti.LineInfo().CharOffset
		parts = append(parts, m.form.View(m.width, m.input.ti.Value(), cursorOff))
	}

	// 채팅 영역과 thinking 표시 사이에 항상 빈 줄을 하나 예약한다.
	// \033[2K: 빈 줄이 아니라 라인 전체를 지우는 이스케이프를 사용해 이전 프레임의
	// 잔상(parallel panel 상단 테두리 등)이 남지 않도록 한다.
	parts = append(parts, "\033[2K")
	// shimmer 라인은 항상 인풋박스 바로 위에 고정 예약한다.
	// busy 중 아닐 때는 라인 클리어로 유지해 레이아웃이 흔들리지 않게 한다.
	shimmerLine := "\033[2K"
	if m.busy && m.shimmer.active {
		shimmerLine = m.shimmer.View()
	}
	parts = append(parts, shimmerLine)
	borderColor := ColorPromptBorder
	if m.busy {
		borderColor = ColorDim
	}
	switch {
	case m.privilege.active:
		parts = append(parts, renderInputBox(m.width, m.privilege.render(m.width), ColorWarning, ""))
	case m.form.active:
		// 폼 활성 시: 입력이 폼 박스 안 fake cursor로 표시되므로 하단 입력박스 없음
	default:
		parts = append(parts, renderInputBox(m.width, m.input.View(), borderColor, ""))
	}

	footerSt := footerIdle
	if m.busy {
		footerSt = footerBusy
	}
	if m.privilege.active {
		footerSt = footerSecretPrompt
	}
	if m.form.active {
		footerSt = footerForm
	}
	if footerSt == footerIdle && m.input.hasSlashSuggestions() {
		footerSt = footerSlashMenu
	}
	parts = append(parts, renderFooter(footerSt, m.width))

	parts = append(parts, m.statusBar.View())

	if m.parker != nil && !m.privilege.active && !m.form.active {
		m.updateCursorTarget()
	} else if m.parker != nil {
		m.parker.SetTarget(0, 0, false)
	}

	return "\033[?25l" + strings.Join(parts, "\n") + ansiClearToEnd
}

func (m AppModel) updateCursorTarget() {
	linesAbove := 1
	if m.busy && len(m.streamLines) > 0 {
		linesAbove += len(m.streamLines)
	}
	if m.queue.Len() > 0 {
		linesAbove++
	}
	linesAbove += 2 // 빈 줄 separator + shimmer 라인 (항상 2줄 예약)

	inputH := inputBoxHeight(m.input.View())
	linesBelow := 1
	if m.selection.active {
		linesBelow += m.selection.Height()
	}
	linesBelow++

	totalLines := linesAbove + inputH + linesBelow
	textareaRow := m.input.ti.Line()
	charOffset := m.input.ti.LineInfo().CharOffset

	cursorViewLine := linesAbove + 1 + textareaRow
	linesUp := totalLines - 1 - cursorViewLine
	col := charOffset + 4
	m.parker.SetTarget(linesUp, col, true)
}

func (m AppModel) visibleShellBox() (string, []string, bool) {
	if recent := m.activeTools.MostRecentForeground(); recent != nil && len(recent.shellLines) > 0 {
		title := toolDisplayName(recent.toolName)
		fgCount := m.activeTools.RunningCount() - m.activeTools.BackgroundCount()
		if extra := fgCount - 1; extra > 0 {
			title = fmt.Sprintf("%s (+%d)", title, extra)
		}
		return title, recent.shellLines, true
	}
	if !m.busy {
		if last, ok := m.history.LatestShell(); ok {
			return toolDisplayName(last.toolName), last.shellLines, true
		}
	}
	return "", nil, false
}
