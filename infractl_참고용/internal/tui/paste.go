// Package tui
// File: paste.go
// Description: 멀티라인 붙여넣기 블록 관리
// Responsibility: 긴 붙여넣기 텍스트를 접힌/펼친 상태로 관리하고 렌더링

package tui

import (
	"fmt"
	"strings"
)

// pasteBlock은 사용자가 붙여넣은 멀티라인 텍스트 블록이다.
type pasteBlock struct {
	id        int
	content   string
	lineCount int
	collapsed bool
}

// newPasteBlock은 새 붙여넣기 블록을 생성한다. 기본적으로 접힌 상태이다.
func newPasteBlock(id int, content string) pasteBlock {
	lines := strings.Count(content, "\n") + 1
	return pasteBlock{
		id:        id,
		content:   content,
		lineCount: lines,
		collapsed: true,
	}
}

// Toggle는 접힘/펼침 상태를 전환한다.
func (p *pasteBlock) Toggle() { p.collapsed = !p.collapsed }

// Text는 원본 텍스트를 반환한다.
func (p *pasteBlock) Text() string { return p.content }

// View는 블록을 렌더링한다. 접힌 상태: [Pasted text #N +M lines], 펼침: 전체 내용.
func (p *pasteBlock) View(width int) string {
	if p.collapsed {
		label := fmt.Sprintf("[Pasted text #%d +%d lines]", p.id, p.lineCount)
		return StylePasteBlock.Width(width - 2).Render(label)
	}
	return StylePasteBlock.Width(width - 2).Render(p.content)
}

// pasteMinLines는 블록으로 처리할 최소 줄 수이다.
const pasteMinLines = 3
