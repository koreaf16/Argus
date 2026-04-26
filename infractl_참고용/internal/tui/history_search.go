// Package tui
// File: history_search.go
// Description: 세션/프롬프트 히스토리 검색 UI
// Responsibility: 과거 입력 히스토리를 검색하여 터미널에 표시하고 선택 가능하게 제공

package tui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

const maxSearchResults = 10

// HistoryEntry는 검색 결과 항목이다.
type HistoryEntry struct {
	Input     string
	Timestamp time.Time
	SessionID int64
}

// RunHistorySearch는 히스토리 검색 UI를 실행하고 선택된 항목을 반환한다.
// 빈 문자열 반환 시 선택 없음.
func RunHistorySearch(entries []HistoryEntry, width int) string {
	if len(entries) == 0 {
		tw := newTermWriter()
		tw.Println(StyleCmdBoxDim.Render("  히스토리가 비어있습니다."))
		return ""
	}

	// 검색 모드: raw mode로 키 입력 직접 읽기
	oldState, err := enableRawMode()
	if err != nil {
		return fallbackHistorySearch(entries)
	}
	defer restoreTermMode(oldState)

	tw := newTermWriter()
	tw.HideCursor()
	defer tw.ShowCursor()

	query := ""
	cursor := 0
	filtered := filterEntries(entries, query)

	renderHistoryUI(tw, query, filtered, cursor, width)

	buf := make([]byte, 64)
	for {
		n, err := readKey(buf)
		if err != nil || n == 0 {
			continue
		}

		switch {
		case buf[0] == 13 || buf[0] == 10: // Enter
			tw.ClearLive()
			tw.ShowCursor()
			restoreTermMode(oldState)
			if cursor < len(filtered) {
				return filtered[cursor].Input
			}
			return ""

		case buf[0] == 27 && n >= 3 && buf[1] == '[': // 화살표
			switch buf[2] {
			case 'A': // ↑
				if cursor > 0 {
					cursor--
				}
			case 'B': // ↓
				if cursor < len(filtered)-1 {
					cursor++
				}
			}

		case buf[0] == 27 && n == 1: // ESC
			tw.ClearLive()
			return ""

		case buf[0] == 3: // Ctrl+C
			tw.ClearLive()
			return ""

		case buf[0] == 127 || buf[0] == 8: // Backspace
			if len(query) > 0 {
				query = query[:len(query)-1]
				filtered = filterEntries(entries, query)
				cursor = 0
			}

		case buf[0] >= 32 && buf[0] < 127: // 일반 문자
			query += string(buf[:1])
			filtered = filterEntries(entries, query)
			cursor = 0

		default:
			// UTF-8 멀티바이트 (한글 등)
			if buf[0] >= 0xC0 && n >= 2 {
				query += string(buf[:n])
				filtered = filterEntries(entries, query)
				cursor = 0
			}
		}

		tw.ClearLive()
		renderHistoryUI(tw, query, filtered, cursor, width)
	}
}

// renderHistoryUI는 Gemini CLI 박스 스타일로 히스토리 검색 UI를 렌더링한다.
func renderHistoryUI(tw *termWriter, query string, entries []HistoryEntry, cursor int, width int) {
	innerW := width - 6
	if innerW < 40 {
		innerW = 40
	}

	// 콘텐츠 빌드
	var content strings.Builder
	content.WriteString("\n")

	// 검색 입력 라인
	queryDisplay := query
	if queryDisplay == "" {
		queryDisplay = StyleGeminiSubDesc.Render("(type to filter)")
	} else {
		queryDisplay = StyleGeminiSelected.Render(query)
	}
	content.WriteString(StyleGeminiSubDesc.Render("  Search: ") + queryDisplay + StyleGeminiOption.Render("█") + "\n")
	content.WriteString("\n")

	// 결과 목록
	show := entries
	if len(show) > maxSearchResults {
		show = show[:maxSearchResults]
	}

	if len(show) == 0 {
		content.WriteString(StyleGeminiSubDesc.Render("  No results found.") + "\n")
	} else {
		for i, entry := range show {
			ago := formatTimeAgo(entry.Timestamp)
			text := truncateStr(entry.Input, innerW-20)
			if i == cursor {
				bullet := StyleGeminiBullet.Render("●")
				label := StyleGeminiSelected.Render(fmt.Sprintf("%d. %s", i+1, text))
				content.WriteString(fmt.Sprintf("  %s %s  %s\n", bullet, label, StyleGeminiSubDesc.Render(ago)))
			} else {
				content.WriteString(StyleGeminiOption.Render(fmt.Sprintf("    %d. %s", i+1, text)) +
					"  " + StyleGeminiSubDesc.Render(ago) + "\n")
			}
		}
		if len(entries) > maxSearchResults {
			content.WriteString(StyleGeminiSubDesc.Render(
				fmt.Sprintf("  ... +%d more", len(entries)-maxSearchResults)) + "\n")
		}
	}

	content.WriteString("\n")
	content.WriteString(StyleGeminiHint.Render("  Enter to select · ↑/↓ to navigate · Esc to cancel") + "\n")

	// 박스 렌더링
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorGeminiBox).
		PaddingLeft(1).
		PaddingRight(1).
		Width(innerW + 2)

	rendered := boxStyle.Render(content.String())

	// 헤더 타이틀 삽입
	lines := strings.SplitN(rendered, "\n", 2)
	if len(lines) >= 2 {
		titleLine := " " + StyleGeminiHeader.Render("🔍 History Search") + " "
		firstLineW := lipgloss.Width(lines[0])
		headerLine := buildBoxHeader(titleLine, firstLineW)
		rendered = headerLine + "\n" + lines[1]
	}

	// 줄 수 계산 후 출력
	lineCount := strings.Count(rendered, "\n") + 1
	tw.prevLines = lineCount
	tw.Print(rendered + "\n")
}

// filterEntries는 쿼리로 히스토리를 필터링한다.
func filterEntries(entries []HistoryEntry, query string) []HistoryEntry {
	if query == "" {
		return entries
	}
	q := strings.ToLower(query)
	var result []HistoryEntry
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Input), q) {
			result = append(result, e)
		}
	}
	return result
}

// formatTimeAgo는 시간을 "5분 전", "1시간 전" 등으로 포맷한다.
func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "방금"
	case d < time.Hour:
		return fmt.Sprintf("%d분 전", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d시간 전", int(d.Hours()))
	default:
		days := int(d.Hours()) / 24
		if days == 1 {
			return "어제"
		}
		return fmt.Sprintf("%d일 전", days)
	}
}

// fallbackHistorySearch는 raw mode 실패 시 번호 입력 방식 폴백이다 (Gemini 스타일 적용).
func fallbackHistorySearch(entries []HistoryEntry) string {
	show := entries
	if len(show) > maxSearchResults {
		show = show[:maxSearchResults]
	}
	borderColor := lipgloss.NewStyle().Foreground(ColorGeminiBox)
	fmt.Println(borderColor.Render("╭") + " " + StyleGeminiHeader.Render("🔍 History Search"))
	fmt.Println(borderColor.Render("│"))
	for i, e := range show {
		ago := formatTimeAgo(e.Timestamp)
		fmt.Printf("%s    %d. %s  %s\n",
			borderColor.Render("│"),
			i+1,
			truncateStr(e.Input, 60),
			StyleGeminiSubDesc.Render(ago))
	}
	fmt.Println(borderColor.Render("│"))
	fmt.Print(borderColor.Render("│  ") + StyleGeminiHint.Render("선택 (번호, 빈 입력=취소): "))
	
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	text := strings.TrimSpace(line)
	
	fmt.Println(borderColor.Render("╰" + strings.Repeat("─", 50) + "╯"))
	
	for i, e := range show {
		if text == fmt.Sprintf("%d", i+1) {
			return e.Input
		}
	}
	return ""
}

// readKey는 stdin에서 키 입력을 읽는다 (raw mode에서).
func readKey(buf []byte) (int, error) {
	return readFromStdin(buf)
}
