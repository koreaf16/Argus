// Package tui
// File: tool_overlay.go
// Description: Ctrl+O 도구 이력 스크롤 오버레이 컴포넌트
// Responsibility: 완료된 도구 이력을 스크롤 가능한 풀스크린 오버레이로 표시

package tui

import (
	"fmt"
	"strings"
	"time"
)

const (
	overlayVisibleLines = 20  // 한 번에 표시할 이력 항목 수
	overlayMaxResultLen = 300 // 결과 미리보기 최대 글자 수
)

// toolOverlayState는 Ctrl+O 도구 이력 오버레이 상태이다.
type toolOverlayState struct {
	active  bool
	scrollY int // 현재 스크롤 위치 (항목 인덱스 기준)
}

// isActive는 오버레이가 활성화되어 있는지 반환한다.
func (o *toolOverlayState) isActive() bool {
	return o.active
}

// open은 오버레이를 열고 스크롤을 맨 아래(최신)로 이동한다.
func (o *toolOverlayState) open(histLen int) {
	o.active = true
	// 최신 항목이 보이도록 스크롤 위치 설정
	o.scrollY = max(0, histLen-overlayVisibleLines)
}

// close는 오버레이를 닫는다.
func (o *toolOverlayState) close() {
	o.active = false
}

// scrollUp은 스크롤을 위로 이동한다.
func (o *toolOverlayState) scrollUp() {
	if o.scrollY > 0 {
		o.scrollY--
	}
}

// scrollDown은 스크롤을 아래로 이동한다.
func (o *toolOverlayState) scrollDown(histLen int) {
	maxScroll := max(0, histLen-overlayVisibleLines)
	if o.scrollY < maxScroll {
		o.scrollY++
	}
}

// View는 오버레이를 렌더링한다.
func (o *toolOverlayState) View(hist *toolHistory, width int) string {
	if !o.active || hist.Len() == 0 {
		return renderEmptyOverlay(width)
	}

	var b strings.Builder

	// 헤더
	title := StyleCmdBoxToolName.Render("  도구 실행 이력")
	hint := StyleCmdBoxDim.Render("↑↓ 스크롤  Esc/Ctrl+O 닫기")
	headerPad := max(0, width-visibleLen(title)-visibleLen(hint)-2)
	b.WriteString(title + strings.Repeat(" ", headerPad) + hint + "\n")
	b.WriteString(StyleCmdBoxBorder.Render(strings.Repeat("─", max(0, width-2))) + "\n")

	// 항목 표시
	start := o.scrollY
	end := min(start+overlayVisibleLines, hist.Len())
	if end == 0 {
		b.WriteString(StyleCmdBoxDim.Render("  (이력 없음)") + "\n")
	}

	for i := start; i < end; i++ {
		entry, ok := hist.Get(i)
		if !ok {
			continue
		}
		b.WriteString(renderHistoryEntry(entry, width))
		b.WriteString("\n")
	}

	// 스크롤 표시
	if hist.Len() > overlayVisibleLines {
		shown := fmt.Sprintf("  %d–%d / %d", start+1, end, hist.Len())
		b.WriteString(StyleCmdBoxDim.Render(shown))
	}

	return b.String()
}

// renderHistoryEntry는 단일 이력 항목을 한 줄로 렌더링한다.
func renderHistoryEntry(e toolHistoryEntry, width int) string {
	// 상태 아이콘
	icon := ""
	if !e.success {
		icon = StyleError.Render("✗") + " "
	}

	// 도구명 + 대상
	name := StyleCmdBoxToolName.Render(toolDisplayName(e.toolName))
	if e.target != "" && e.target != "localhost" {
		name += StyleCmdBoxDim.Render(" → ") + StyleCmdBoxTarget.Render(e.target)
	}

	// 시간 정보
	dur := StyleCmdBoxDim.Render(formatElapsedShort(e.duration))
	ts := StyleCmdBoxDim.Render(formatRelativeTime(e.finishedAt))

	// 결과 미리보기 (첫 줄, 잘라내기)
	preview := toolSummaryPreview(e.toolName, nil, e.result, e.metadataJSON, e.success)
	if len(preview) > overlayMaxResultLen {
		preview = preview[:overlayMaxResultLen-3] + "..."
	}
	previewStr := ""
	if preview != "" {
		previewStr = "  " + StyleCmdBoxDim.Render(preview)
	}

	line1 := fmt.Sprintf("  %s %s  %s  %s", icon, name, dur, ts)
	if len(line1)+len(previewStr) > width-2 && previewStr != "" {
		return line1 + "\n" + previewStr
	}
	if previewStr != "" {
		return line1 + previewStr
	}
	return line1
}

func renderEmptyOverlay(width int) string {
	title := StyleCmdBoxToolName.Render("  도구 실행 이력")
	hint := StyleCmdBoxDim.Render("Esc/Ctrl+O 닫기")
	b := title + "  " + hint + "\n"
	b += StyleCmdBoxBorder.Render(strings.Repeat("─", max(0, width-2))) + "\n"
	b += StyleCmdBoxDim.Render("  (이력 없음)\n")
	return b
}

// formatRelativeTime은 시각을 "3s ago", "2m ago" 형식으로 반환한다.
func formatRelativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

// firstNonEmptyLine은 문자열에서 공백이 아닌 첫 번째 줄을 반환한다.
func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

// visibleLen은 ANSI 시퀀스를 제외한 표시 길이를 대략 계산한다.
func visibleLen(s string) int {
	// 간단 구현: ANSI escape 무시하고 len 사용 (정확하지 않지만 레이아웃 목적엔 충분)
	inEsc := false
	count := 0
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
			}
			continue
		}
		count++
	}
	return count
}
