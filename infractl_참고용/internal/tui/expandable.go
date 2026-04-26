// Package tui
// File: expandable.go
// Description: 긴 텍스트의 접기/펼치기 렌더링
// Responsibility: 에러 메시지, 도구 출력 등을 기본 N줄로 제한하고 펼치기 지원

package tui

import (
	"fmt"
	"strings"
)

const (
	defaultCollapsedLines = 3  // 기본 접힘 줄 수
	expandHintKey         = "Enter" // 펼치기 키
)

// ExpandableText는 접기/펼치기 가능한 텍스트이다.
type ExpandableText struct {
	content   string
	maxLines  int
	expanded  bool
}

// NewExpandableText는 새 ExpandableText를 생성한다.
func NewExpandableText(content string, maxLines int) *ExpandableText {
	if maxLines <= 0 {
		maxLines = defaultCollapsedLines
	}
	return &ExpandableText{content: content, maxLines: maxLines}
}

// Toggle은 접기/펼치기 상태를 전환한다.
func (e *ExpandableText) Toggle() {
	e.expanded = !e.expanded
}

// IsExpandable은 접기가 가능한지 (줄 수가 maxLines 초과) 반환한다.
func (e *ExpandableText) IsExpandable() bool {
	return strings.Count(e.content, "\n")+1 > e.maxLines
}

// View는 현재 상태에 따라 렌더링된 텍스트를 반환한다.
func (e *ExpandableText) View() string {
	if !e.IsExpandable() || e.expanded {
		return e.content
	}
	return collapseText(e.content, e.maxLines)
}

// collapseText는 텍스트를 maxLines로 제한하고 힌트를 추가한다.
func collapseText(content string, maxLines int) string {
	lines := strings.Split(content, "\n")
	if len(lines) <= maxLines {
		return content
	}

	preview := strings.Join(lines[:maxLines], "\n")
	remaining := len(lines) - maxLines
	hint := StyleCmdBoxDim.Render(
		fmt.Sprintf("  ... +%d lines (%s to expand)", remaining, expandHintKey))
	return preview + "\n" + hint
}

// RenderError는 에러 메시지를 접기 가능한 형태로 렌더링한다.
// 3줄 이하면 그대로, 초과면 접어서 표시.
func RenderError(errText string) string {
	bracket := StyleResponseBracket.Render("  ⎿  ")
	errLabel := StyleError.Render("Error")

	lines := strings.Split(errText, "\n")
	if len(lines) <= defaultCollapsedLines {
		// 짧은 에러: 그대로 표시
		var sb strings.Builder
		sb.WriteString(bracket + errLabel + "\n")
		for _, line := range lines {
			sb.WriteString("     " + StyleError.Render(line) + "\n")
		}
		return sb.String()
	}

	// 긴 에러: 접어서 표시
	var sb strings.Builder
	sb.WriteString(bracket + errLabel + "\n")
	for _, line := range lines[:defaultCollapsedLines] {
		sb.WriteString("     " + StyleError.Render(line) + "\n")
	}
	remaining := len(lines) - defaultCollapsedLines
	sb.WriteString(StyleCmdBoxDim.Render(
		fmt.Sprintf("     ... +%d lines", remaining)) + "\n")
	return sb.String()
}

// RenderErrorFull은 에러 메시지를 전체 표시한다 (펼침 상태).
func RenderErrorFull(errText string) string {
	bracket := StyleResponseBracket.Render("  ⎿  ")
	errLabel := StyleError.Render("Error")

	var sb strings.Builder
	sb.WriteString(bracket + errLabel + "\n")
	for _, line := range strings.Split(errText, "\n") {
		sb.WriteString("     " + StyleError.Render(line) + "\n")
	}
	return sb.String()
}
