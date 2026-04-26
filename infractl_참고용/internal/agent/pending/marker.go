// Package pending
// File: marker.go
// Description: 도중 추가 입력을 LLM 메시지 형식으로 변환하는 포매터
// Responsibility: FormatInjection — 마커 프리픽스 포맷팅 단일 책임

package pending

import "strings"

const marker = "[도중 추가 입력]"

// FormatInjection은 drain된 입력들을 단일 user-role 메시지 본문으로 변환한다.
// 여러 건은 개행으로 합쳐 하나의 마커 블록으로 묶인다.
// 입력이 없으면 빈 문자열을 반환한다.
func FormatInjection(inputs []PendingInput) string {
	if len(inputs) == 0 {
		return ""
	}
	lines := make([]string, len(inputs))
	for i, inp := range inputs {
		lines[i] = inp.Text
	}
	return marker + "\n" + strings.Join(lines, "\n")
}
