// Package tui
// File: chatview_render.go
// Description: chatItem을 줄 슬라이스로 변환하는 렌더링 로직
// Responsibility: 아이템 종류별 렌더링, 선택 스타일 적용, 보조 유틸

package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Update는 스피너 tick과 박스 업데이트를 처리한다.
func (cv *chatView) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	for i := range cv.items {
		switch cv.items[i].kind {
		case kindThinking:
			if cv.items[i].isThinking {
				var cmd tea.Cmd
				cv.items[i].spinner, cmd = cv.items[i].spinner.Update(msg)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
				cv.markDirty(i)
			}
		case kindBox:
			if cv.items[i].box != nil && cv.items[i].box.status == boxRunning {
				cmd := cv.items[i].box.Update(msg)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
				cv.markDirty(i)
			}
		}
	}
	if cv.pendingRefresh {
		cv.pendingRefresh = false
		cv.lastRefresh = time.Now()
	}
	if len(cmds) > 0 {
		cv.refresh()
	}
	return tea.Batch(cmds...)
}

// View는 캐싱된 뷰포트 문자열을 반환한다.
func (cv *chatView) View() string {
	return cv.viewCache
}

// renderItemLines는 단일 chatItem을 줄 슬라이스로 변환한다.
func (cv *chatView) renderItemLines(idx int, item chatItem) []string {
	isSelected := idx == cv.selectedIdx
	var content string

	switch item.kind {
	case kindUser:
		// Claude CLI 스타일: ❯ You 헤더 + 배경색 본문
		header := StyleInputPrompt.Render("❯ ") + lipgloss.NewStyle().Bold(true).Render("You")
		body := StyleUserMsg.Render("  " + item.text)
		content = "\n" + header + "\n" + body + "\n"

	case kindThinking:
		if !item.isThinking {
			return nil
		}
		dots := thinkingDots(time.Since(item.startTime))
		header := fmt.Sprintf("%s Thinking%-3s", item.spinner.View(), dots)
		content = "\n" + StyleThinking.Render(header)

	case kindAssistant:
		if item.assistant == "" && len(item.renderedLines) == 0 {
			return nil
		}
		if len(item.renderedLines) > 0 {
			lines := item.renderedLines
			if isSelected {
				var styled []string
				for _, l := range lines {
					styled = append(styled, applySelection(l))
				}
				return addPrefix(styled, "\n")
			}
			return addPrefix(lines, "\n")
		}
		content = "\n" + item.assistant

	case kindBox:
		if item.box == nil {
			return nil
		}
		rendered := item.box.View(cv.width)
		return strings.Split(rendered, "\n")

	case kindError:
		// Claude CLI 스타일: 좌측 빨간 border
		content = "\n" + lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(ColorError).
			PaddingLeft(1).
			Render("Error: "+item.text) + "\n"
	}

	if content == "" {
		return nil
	}

	if isSelected {
		content = applySelection(content)
	}

	return strings.Split(content, "\n")
}

// thinkingDots는 경과 시간에 따라 점(.) 개수를 애니메이션 효과로 변환한다.
func thinkingDots(elapsed time.Duration) string {
	phase := int(elapsed.Milliseconds()/500) % 3
	return strings.Repeat(".", phase+1)
}

// applySelection은 선택 상태 스타일(왼쪽 보더)을 적용한다.
func applySelection(content string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(ColorClaude).
		PaddingLeft(1).
		Render(content)
}

// addPrefix는 줄 슬라이스 앞에 prefix를 분할하여 추가한다.
func addPrefix(lines []string, prefix string) []string {
	result := make([]string, 0, len(lines)+1)
	result = append(result, strings.Split(prefix, "\n")...)
	result = append(result, lines...)
	return result
}
