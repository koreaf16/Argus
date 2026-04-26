// Package tui
// File: parallel_panel.go
// Description: 병렬 실행 중인 도구들을 하나의 통합 패널로 렌더링
// Responsibility: 여러 도구가 동시에 실행될 때 단일 그룹 박스로 표시

package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// renderParallelActivePanel은 여러 포그라운드 도구가 실행 중일 때 통합 패널을 렌더링한다.
func renderParallelActivePanel(fgTools []*activeToolState, totalElapsed time.Duration, width int) string {
	boxW := width - 4
	if boxW < 30 {
		boxW = 30
	}
	innerW := boxW - 4

	var bodyLines []string
	for _, st := range fgTools {
		toolElapsed := time.Since(st.startTime)
		label := toolShimmerLabel(st.toolName, st.args)
		if st.target != "" {
			label += " @" + st.target
		}
		row := fmt.Sprintf("%s %-28s %s",
			StyleClaude().Render("⠙"),
			truncateStr(label, 28),
			StyleCmdBoxDim.Render(formatElapsedShort(toolElapsed)),
		)
		bodyLines = append(bodyLines, truncateShellLine(row, innerW))
	}

	body := strings.Join(bodyLines, "\n")
	rendered := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorClaude).
		PaddingLeft(1).
		PaddingRight(1).
		Width(boxW).
		Render(body)

	title := fmt.Sprintf("Parallel Tools (%d) · %s", len(fgTools), formatElapsedShort(totalElapsed))
	splitLines := strings.Split(rendered, "\n")
	if len(splitLines) > 0 {
		outerW := lipgloss.Width(splitLines[0])
		if outerW < 6 {
			outerW = boxW + 4
		}
		splitLines[0] = buildShellTopBorderLine(title, outerW, ColorClaude)
	}
	return strings.Join(splitLines, "\n")
}
