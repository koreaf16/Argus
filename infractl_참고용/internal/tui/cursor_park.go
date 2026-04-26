// Package tui
// File: cursor_park.go
// Description: BubbleTea 인라인 모드 프레임 후 커서를 입력 캐럿 위치로 파킹하는 io.Writer 래퍼
// Responsibility: IME 조합 문자가 커서 위치에 표시되도록 하드웨어 커서를 textarea 캐럿에 고정

package tui

import (
	"fmt"
	"os"
	"sync"
)

// CursorParkWriter는 BubbleTea 렌더 출력을 가로채어
// 프레임 종료 후 커서를 textarea 캐럿 위치로 이동시키는 writer다.
//
// tea.WithOutput(parker)으로 BubbleTea 프로그램에 주입한다.
// github.com/charmbracelet/x/term.File 인터페이스(Read/Write/Close/Fd)를
// 구현하여 BubbleTea가 터미널 크기를 올바르게 감지할 수 있도록 한다.
//
// 파킹 메커니즘:
//  1. 프레임 끝 \r 감지 → \033[s 로 위치 저장 → textarea로 커서 이동 (IME 위치 고정)
//  2. 다음 Write() 호출 시 → \033[u 로 저장된 위치 복원 → BubbleTea의 cursor-up이 올바른 위치에서 시작
type CursorParkWriter struct {
	f       *os.File // 실제 출력 파일 (보통 os.Stdout)
	mu      sync.Mutex
	linesUp int  // 마지막 줄에서 커서까지 올라갈 줄 수 (0이면 이동 없음)
	col     int  // 커서 열 (1-indexed ANSI, 0이면 이동 없음)
	active  bool // true면 커서 파킹 활성화 (idle 상태에서만)
	parked  bool // 커서가 파킹된 상태 — 다음 Write()에서 \033[u 복원 필요
}

// NewCursorParkWriter는 새 CursorParkWriter를 생성한다.
func NewCursorParkWriter(f *os.File) *CursorParkWriter {
	return &CursorParkWriter{f: f}
}

// Write는 BubbleTea 렌더 출력을 파일에 전달한다.
// BubbleTea 인라인 모드는 프레임 끝에 \r을 쓰므로, 이를 감지하여 커서를 파킹한다.
//
// 파킹 상태(parked=true)이면 먼저 \033[u 로 커서를 복원한다.
// 이렇게 하면 BubbleTea의 다음 프레임 cursor-up이 올바른 기준점(마지막 줄)에서 시작된다.
func (w *CursorParkWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	shouldRestore := w.parked
	if shouldRestore {
		w.parked = false
	}
	w.mu.Unlock()

	// 파킹 해제: 숨긴 후 복원 → 복원 순간 커서가 statusbar에 깜빡이지 않음
	if shouldRestore {
		if _, err := fmt.Fprint(w.f, "\033[?25l\033[u"); err != nil {
			return 0, err
		}
	}

	n, err := w.f.Write(p)
	if err != nil {
		return n, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	// BubbleTea 인라인 모드는 각 프레임을 \r로 종료한다
	if w.active && len(p) > 0 && p[len(p)-1] == '\r' {
		w.parkCursor()
	}
	return n, nil
}

// Read는 term.File 인터페이스 구현 — 파일에서 읽기 위임.
func (w *CursorParkWriter) Read(p []byte) (int, error) {
	return w.f.Read(p)
}

// Close는 term.File 인터페이스 구현 — 파일 닫기 위임.
func (w *CursorParkWriter) Close() error {
	return w.f.Close()
}

// Fd는 term.File 인터페이스 구현 — 파일 디스크립터(Windows: HANDLE) 반환.
// BubbleTea가 터미널 크기 감지 및 VT 처리 활성화에 사용한다.
func (w *CursorParkWriter) Fd() uintptr {
	return w.f.Fd()
}

// SetTarget은 다음 프레임 렌더 후 커서를 이동할 위치를 설정한다.
// View()가 호출될 때마다 계산된 위치로 업데이트해야 한다.
func (w *CursorParkWriter) SetTarget(linesUp, col int, active bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.linesUp = linesUp
	w.col = col
	w.active = active
}

// parkCursor는 커서를 목표 위치로 이동하고 표시한다. mu를 보유한 상태에서 호출한다.
// \033[s 로 현재 위치(\r 행)를 저장한 뒤 textarea로 이동한다.
// 다음 Write() 에서 \033[u 로 복원하여 BubbleTea의 cursor-up 기준점을 유지한다.
func (w *CursorParkWriter) parkCursor() {
	// 현재 위치(\r 행 = BubbleTea가 다음 프레임 기준으로 쓸 위치) 저장
	fmt.Fprint(w.f, "\033[s")
	if w.linesUp > 0 {
		fmt.Fprintf(w.f, "\033[%dA", w.linesUp)
	}
	if w.col > 0 {
		fmt.Fprintf(w.f, "\033[%dG", w.col)
	}
	fmt.Fprint(w.f, "\033[?25h")
	w.parked = true
}
