// Package tui
// File: anim.go
// Description: TUI 애니메이션 상수 및 스피너 프레임 집중 관리
// Responsibility: 틱 주기, 스피너 프레임, 기타 애니메이션 파라미터를 단일 위치에서 정의

package tui

import "time"

// AnimConfig는 전역 애니메이션 설정이다.
var AnimConfig = struct {
	// SpinnerInterval은 스피너/애니메이션 공통 틱 주기.
	SpinnerInterval time.Duration
	// ShimmerInterval은 shimmer 그라디언트 갱신 주기 (고주파 유지).
	ShimmerInterval time.Duration
	// ShellDotsInterval은 shell_box 점(.) 애니메이션 주기.
	ShellDotsInterval time.Duration
}{
	SpinnerInterval:   100 * time.Millisecond,
	ShimmerInterval:   50 * time.Millisecond,
	ShellDotsInterval: 100 * time.Millisecond,
}

// BrailleFrames는 공용 Braille 스피너 프레임이다.
var BrailleFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// brailleFrame은 경과 시간에 따른 Braille 프레임 인덱스를 반환한다.
func brailleFrame(elapsed time.Duration) string {
	idx := int(elapsed.Milliseconds()/100) % len(BrailleFrames)
	return BrailleFrames[idx]
}
