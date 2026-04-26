// Package tui
// File: inputbox.go
// Description: Claude CLI 스타일 라운드 border 입력 박스 렌더링
// Responsibility: 입력창의 상단/하단 가로선과 좌측 border를 포함한 시각적 프레임 제공

package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/ansi"
)

// border 문자 상수
const (
	borderTopLeft    = "─"
	borderBottomLeft = "─"
	borderHorizontal = "─"
)

// renderInputBox는 입력 내용을 border로 감싸서 렌더링한다.
// 좌측 수직선 없이 상/하단 가로선만 표시하고 1칸 들여쓰기.
//
//	──────────────────────── borderText
//	 > 입력 텍스트
//	────────────────────────
func renderInputBox(width int, inputView string, borderColor lipgloss.Color, borderText string) string {
	if width < 10 {
		return inputView
	}

	top := buildTopBorder(width, borderColor, borderText)
	bottom := buildBottomBorder(width, borderColor)

	// 입력 내용을 줄 단위로 처리 (좌측 수직선 없음, 1칸 들여쓰기)
	lines := strings.Split(inputView, "\n")
	var middle strings.Builder
	for i, line := range lines {
		middle.WriteString(" " + line)
		if i < len(lines)-1 {
			middle.WriteString("\n")
		}
	}

	return top + "\n" + middle.String() + "\n" + bottom
}

// buildTopBorder는 상단 border를 생성한다.
// borderText가 있으면 우상단에 표시한다.
func buildTopBorder(width int, borderColor lipgloss.Color, borderText string) string {
	colorFn := lipgloss.NewStyle().Foreground(borderColor)
	corner := colorFn.Render(borderTopLeft)

	if borderText == "" {
		lineWidth := max(width-1, 0)
		return corner + colorFn.Render(strings.Repeat(borderHorizontal, lineWidth))
	}

	// borderText를 우측에 배치
	textWidth := ansi.PrintableRuneWidth(borderText)
	lineWidth := max(width-1-textWidth-2, 4)
	return corner + colorFn.Render(strings.Repeat(borderHorizontal, lineWidth)) +
		" " + borderText + " " +
		colorFn.Render(strings.Repeat(borderHorizontal, 1))
}

// buildBottomBorder는 하단 border를 생성한다.
func buildBottomBorder(width int, borderColor lipgloss.Color) string {
	colorFn := lipgloss.NewStyle().Foreground(borderColor)
	corner := colorFn.Render(borderBottomLeft)

	lineWidth := max(width-1, 0)
	return corner + colorFn.Render(strings.Repeat(borderHorizontal, lineWidth))
}

// inputBoxHeight는 입력 박스가 차지하는 줄 수를 반환한다.
// top border(1) + 입력 줄 수 + bottom border(1)
func inputBoxHeight(inputView string) int {
	lines := strings.Count(inputView, "\n") + 1
	return lines + 2 // top + content + bottom
}
