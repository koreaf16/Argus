package tui

import (
	"fmt"
	"os"
	"sync"
)

const (
	ansiHideCursor = "\x1b[?25l"
	ansiShowCursor = "\x1b[?25h"
	ansiSaveCursor = "\x1b7" // DEC Save Cursor
	ansiLoadCursor = "\x1b8" // DEC Restore Cursor
)

// CursorParkWriter forwards Bubble Tea output while re-parking the terminal
// cursor to the textarea caret after each rendered frame.
//
// Implements github.com/charmbracelet/x/term.File (Read/Write/Close/Fd) so
// Bubble Tea can detect terminal size through the wrapped writer.
//
// 파킹 메커니즘:
//  1. 프레임 끝 \r 감지 → DECSC로 위치 저장 → textarea로 커서 이동 (IME 위치 고정)
//  2. 다음 Write() 호출 시 → 커서 숨김 + DECRC로 저장된 위치 복원 → BubbleTea의 cursor-up이 올바른 위치에서 시작
type CursorParkWriter struct {
	f       *os.File
	mu      sync.Mutex
	linesUp int
	col     int
	active  bool
	parked  bool
}

func NewCursorParkWriter(f *os.File) *CursorParkWriter {
	return &CursorParkWriter{f: f}
}

func (w *CursorParkWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	shouldRestore := w.parked
	if shouldRestore {
		w.parked = false
	}
	w.mu.Unlock()

	// 파킹 해제: 커서를 숨긴 후 저장된 위치(프레임 하단)로 복원
	// 숨기지 않으면 복원 순간 커서가 하단에 깜빡임
	if shouldRestore {
		if _, err := fmt.Fprint(w.f, ansiHideCursor+ansiLoadCursor); err != nil {
			return 0, err
		}
	}

	n, err := w.f.Write(p)
	if err != nil {
		return n, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	// BubbleTea v0.x는 \r로, v1.x 인라인 모드는 \x1b[<n>D (CursorLeft)로 프레임을 종료한다
	if w.active && isFrameEnd(p) {
		w.parkCursorLocked()
	}
	return n, nil
}

// isFrameEnd는 p가 BubbleTea 프레임 종료 시퀀스로 끝나는지 확인한다.
// v0.x: \r / v1.x inline: \x1b[<n>D (ansi.CursorLeft)
func isFrameEnd(p []byte) bool {
	n := len(p)
	if n == 0 {
		return false
	}
	if p[n-1] == '\r' {
		return true
	}
	// \x1b[<digits>D 패턴 감지
	if p[n-1] != 'D' {
		return false
	}
	i := n - 2
	for i >= 0 && p[i] >= '0' && p[i] <= '9' {
		i--
	}
	return i >= 1 && p[i] == '[' && p[i-1] == '\x1b'
}

// Read implements term.File.
func (w *CursorParkWriter) Read(p []byte) (int, error) {
	return w.f.Read(p)
}

// Close implements term.File.
func (w *CursorParkWriter) Close() error {
	return w.f.Close()
}

// Fd implements term.File — returns the underlying file descriptor so Bubble
// Tea can query terminal size and enable VT processing.
func (w *CursorParkWriter) Fd() uintptr {
	return w.f.Fd()
}

// SetTarget sets the caret position to park the cursor after each frame.
// Called from View() every render cycle.
func (w *CursorParkWriter) SetTarget(linesUp, col int, active bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.linesUp = linesUp
	w.col = col
	w.active = active
}

// parkCursorLocked saves the bottom-of-frame position (DECSC), moves to the
// textarea caret, and shows the cursor so Windows IME can anchor to it.
// Must be called with mu held.
func (w *CursorParkWriter) parkCursorLocked() {
	fmt.Fprint(w.f, ansiSaveCursor)
	if w.linesUp > 0 {
		fmt.Fprintf(w.f, "\x1b[%dA", w.linesUp)
	}
	if w.col > 0 {
		fmt.Fprintf(w.f, "\x1b[%dG", w.col)
	}
	fmt.Fprint(w.f, ansiShowCursor)
	w.parked = true
}
