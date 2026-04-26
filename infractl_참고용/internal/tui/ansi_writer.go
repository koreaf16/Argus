// Package tui
// File: ansi_writer.go
// Description: ANSI 이스케이프 시퀀스 기반 터미널 출력 헬퍼
// Responsibility: 커서 이동, 줄 삭제, 인플레이스 업데이트 등 저수준 터미널 제어

package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ANSI 이스케이프 시퀀스 상수
const (
	ansiCursorUp    = "\033[%dA" // 커서 N줄 위로
	ansiClearToEnd  = "\033[J"   // 커서 위치부터 화면 끝까지 삭제
	ansiClearLine   = "\033[2K"  // 현재 줄 전체 삭제
	ansiCarriageRet = "\r"       // 줄 시작으로 이동
	ansiHideCursor  = "\033[?25l"
	ansiShowCursor  = "\033[?25h"
)

// termWriter는 터미널에 직접 쓰는 writer이다.
type termWriter struct {
	prevLines int // 이전 렌더링의 줄 수 (덮어쓰기용)
}

// newTermWriter는 새 termWriter를 생성한다.
func newTermWriter() *termWriter {
	return &termWriter{}
}

// Print는 텍스트를 stdout에 출력한다.
func (w *termWriter) Print(s string) {
	fmt.Fprint(os.Stdout, s)
}

// Println은 텍스트를 stdout에 출력하고 개행한다.
func (w *termWriter) Println(s string) {
	fmt.Fprintln(os.Stdout, s)
}

// PrintReplace는 이전 출력을 지우고 새 내용으로 교체한다.
// 스트리밍 마크다운, shimmer 등 인플레이스 업데이트에 사용.
func (w *termWriter) PrintReplace(content string) {
	// 이전 출력 지우기
	if w.prevLines > 0 {
		fmt.Fprintf(os.Stdout, ansiCursorUp, w.prevLines)
		fmt.Fprint(os.Stdout, ansiClearToEnd)
	}

	// 새 내용 출력
	fmt.Fprint(os.Stdout, content)
	if !strings.HasSuffix(content, "\n") {
		fmt.Fprint(os.Stdout, "\n")
	}

	// 줄 수 기록 (lipgloss.Height를 사용하여 줄 바꿈 고려)
	w.prevLines = lipgloss.Height(content)
	if !strings.HasSuffix(content, "\n") {
		w.prevLines++
	}
}

// ClearLive는 라이브 영역을 지우고 줄 카운트를 리셋한다.
func (w *termWriter) ClearLive() {
	if w.prevLines > 0 {
		fmt.Fprintf(os.Stdout, ansiCursorUp, w.prevLines)
		fmt.Fprint(os.Stdout, ansiClearToEnd)
		w.prevLines = 0
	}
}

// ResetLines는 줄 카운트만 리셋한다 (새 섹션 시작 시).
func (w *termWriter) ResetLines() {
	w.prevLines = 0
}

// HideCursor는 커서를 숨긴다.
func (w *termWriter) HideCursor() {
	fmt.Fprint(os.Stdout, ansiHideCursor)
}

// ShowCursor는 커서를 보인다.
func (w *termWriter) ShowCursor() {
	fmt.Fprint(os.Stdout, ansiShowCursor)
}

// OverwriteLine은 현재 줄을 덮어쓴다 (shimmer/spinner용).
func (w *termWriter) OverwriteLine(s string) {
	fmt.Fprint(os.Stdout, ansiCarriageRet+ansiClearLine+s)
}
