// Package tui
// File: term_raw.go
// Description: 터미널 raw mode 전환 헬퍼
// Responsibility: Select UI 등에서 키 입력을 직접 읽기 위한 터미널 모드 제어

package tui

import (
	"os"

	"golang.org/x/term"
)

// enableRawMode는 터미널을 raw mode로 전환한다.
// 반환된 state로 restoreTermMode를 호출하여 복원한다.
func enableRawMode() (*term.State, error) {
	fd := int(os.Stdin.Fd())
	return term.MakeRaw(fd)
}

// restoreTermMode는 터미널을 이전 상태로 복원한다.
func restoreTermMode(oldState *term.State) {
	if oldState != nil {
		fd := int(os.Stdin.Fd())
		term.Restore(fd, oldState)
	}
}

// readFromStdin은 stdin에서 바이트를 읽는다 (raw mode에서 사용).
func readFromStdin(buf []byte) (int, error) {
	return os.Stdin.Read(buf)
}
