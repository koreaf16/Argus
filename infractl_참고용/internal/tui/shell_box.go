// Package tui
// File: shell_box.go
// Description: shell_exec / file_transfer 도구의 Gemini CLI 스타일 박스 렌더러
// Responsibility: 상태별 테두리 색상, 상단 커맨드 헤더, 하단 메타 라인 렌더링

package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

const (
	shellBoxMaxLines     = 10
	shellBoxDisplayLines = 10
)

type boxIcon int

const (
	boxIconRunning boxIcon = iota
	boxIconDone
	boxIconFailed
	boxIconGuard
	boxIconCancelled
)

func isShellBoxTool(name string) bool {
	return name == "shell_exec" || name == "file_transfer"
}

func renderShellBoxCompleted(name string, args map[string]any, lines []string, totalLines int, duration time.Duration, success bool, width int, metadataJSON string) string {
	icon := boxIconDone
	if !success {
		icon = boxIconFailed
	}
	return renderShellBoxContent(name, args, lines, totalLines, icon, duration, width, metadataJSON)
}

func renderShellBoxGuard(name string, args map[string]any, lines []string, totalLines int, duration time.Duration, width int) string {
	return renderShellBoxContent(name, args, lines, totalLines, boxIconGuard, duration, width, "")
}

func renderShellBoxCancelled(name string, args map[string]any, lines []string, totalLines int, duration time.Duration, width int) string {
	return renderShellBoxContent(name, args, lines, totalLines, boxIconCancelled, duration, width, "")
}

func renderShellBoxRunning(name string, args map[string]any, lines []string, totalLines int, elapsed time.Duration, width int) string {
	return renderShellBoxContent(name, args, lines, totalLines, boxIconRunning, elapsed, width, "")
}

func renderShellBoxContent(name string, args map[string]any, outputLines []string, totalLines int, icon boxIcon, dur time.Duration, width int, metadataJSON string) string {
	if name == "discover_services" || name == "system_info" || name == "network_info" {
		return renderParallelTaskBox(name, args, outputLines, icon, dur, width)
	}
	if name == "discover_web_servers" {
		return renderDiscoverWebBox(name, args, outputLines, icon, dur, width)
	}
	if name == "web_search" {
		return renderWebSearchBox(args, outputLines, icon, dur, width)
	}

	boxW := width - 4
	if boxW < 30 {
		boxW = 30
	}
	innerW := boxW - 4

	var bodyLines []string
	if status := shellBoxStatusLine(name, args, totalLines, icon, dur, metadataJSON); status != "" {
		bodyLines = append(bodyLines, status)
		if len(outputLines) > 0 {
			bodyLines = append(bodyLines, StyleSubtle.Render(strings.Repeat("-", innerW)))
		}
	}

	if len(outputLines) > shellBoxDisplayLines {
		hint := fmt.Sprintf("⋯ +%d earlier lines", len(outputLines)-shellBoxDisplayLines)
		bodyLines = append(bodyLines, StyleCmdBoxDim.Render(hint))
		outputLines = outputLines[len(outputLines)-shellBoxDisplayLines:]
	}
	for _, line := range outputLines {
		bodyLines = append(bodyLines, truncateShellLine(line, innerW))
	}

	if icon == boxIconRunning && len(outputLines) == 0 {
		spinner := brailleFrame(dur)
		bodyLines = append(bodyLines, StyleClaude().Render("  "+spinner))
	}

	if icon != boxIconRunning {
		bodyLines = append(bodyLines, StyleSubtle.Render(strings.Repeat("─", innerW)))
		bodyLines = append(bodyLines, buildShellMetaLine(icon, dur, totalLines, metadataJSON))
	} else if dur > 0 {
		bodyLines = append(bodyLines, StyleCmdBoxDim.Render(formatElapsedShort(dur)+" elapsed"))
	}

	body := strings.Join(bodyLines, "\n")
	bc := shellBoxBorderColor(icon)

	rendered := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(bc).
		PaddingLeft(1).
		PaddingRight(1).
		Width(boxW).
		Render(body)

	// 상단 테두리 줄을 "╭─ $ command ─...─╮" 로 교체
	cmd := shellBoxCommandStr(name, args)
	if cmd != "" {
		splitLines := strings.Split(rendered, "\n")
		if len(splitLines) > 0 {
			outerW := lipgloss.Width(splitLines[0])
			if outerW < 6 {
				outerW = boxW + 4
			}
			splitLines[0] = buildShellTopBorderLine(cmd, outerW, bc)
		}
		rendered = strings.Join(splitLines, "\n")
	}

	return rendered
}

// shellBoxBorderColor는 boxIcon 상태에 따른 테두리 색상을 반환한다.
func shellBoxBorderColor(icon boxIcon) lipgloss.Color {
	switch icon {
	case boxIconDone:
		return ColorSuccess
	case boxIconFailed:
		return ColorError
	case boxIconGuard:
		return ColorWarning
	case boxIconCancelled:
		return ColorPlanRequest
	default:
		return ColorClaude
	}
}

// buildShellTopBorderLine은 "╭─ $ cmd ─...─╮" 형태의 상단 테두리 줄을 반환한다.
func buildShellTopBorderLine(cmd string, totalWidth int, bc lipgloss.Color) string {
	borderSt := lipgloss.NewStyle().Foreground(bc)
	titleSt := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true)

	title := "$ " + cmd
	const edgeWidth = 6 // "╭─ " (3) + " ─╮" (3)
	maxTitle := totalWidth - edgeWidth
	if maxTitle < 4 {
		maxTitle = 4
	}
	if lipgloss.Width(title) > maxTitle {
		title = "$ " + truncateStr(cmd, maxTitle-2)
	}

	fillWidth := totalWidth - edgeWidth - lipgloss.Width(title)
	if fillWidth < 0 {
		fillWidth = 0
	}
	fill := strings.Repeat("─", fillWidth)

	return borderSt.Render("╭─ ") +
		titleSt.Render(title) +
		borderSt.Render(" "+fill+"╮")
}

// shellBoxCommandStr은 name/args에서 표시용 커맨드 문자열을 추출한다.
func shellBoxCommandStr(name string, args map[string]any) string {
	switch name {
	case "shell_exec":
		if cmd, ok := args["command"].(string); ok {
			return truncateStr(strings.TrimSpace(cmd), 60)
		}
	case "file_transfer":
		if label := shellTaskLabel(name, args); label != "" {
			return truncateStr(label, 60)
		}
	}
	return ""
}

// buildShellMetaLine은 완료/실패 시 하단 메타 정보 줄을 반환한다.
func buildShellMetaLine(icon boxIcon, dur time.Duration, totalLines int, _ string) string {
	var parts []string
	switch icon {
	case boxIconDone:
		parts = append(parts, StyleSuccess.Render("✓ done"))
	case boxIconGuard:
		parts = append(parts, StyleWarning.Render("! guard"))
	case boxIconCancelled:
		label := lipgloss.NewStyle().Foreground(ColorPlanRequest).Render("— cancelled")
		parts = append(parts, label)
	default:
		parts = append(parts, StyleError.Render("✗ failed"))
	}
	if dur > 0 {
		parts = append(parts, StyleCmdBoxDim.Render(formatElapsedShort(dur)))
	}
	if totalLines > 0 {
		parts = append(parts, StyleCmdBoxDim.Render(fmt.Sprintf("%d lines", totalLines)))
	}
	sep := StyleCmdBoxDim.Render(" · ")
	return strings.Join(parts, sep)
}
