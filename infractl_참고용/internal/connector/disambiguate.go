// Package connector
// File: disambiguate.go
// Description: 서버명·서비스타입 충돌 해소 인터페이스 및 타입 정의
// Responsibility: DisambiguateHandler 인터페이스와 DisambiguateOption 타입 제공

package connector

// DisambiguateOption은 충돌 해소 선택지 하나를 나타낸다.
type DisambiguateOption struct {
	Label       string
	Description string
	Preview     string
	Tag         string
	HideOther   bool
}

// DisambiguateHandler는 서버명/서비스타입 충돌 시 사용자 선택 UI를 표시한다.
// TUI 모드에서는 cmd/infractl/wiring.go의 tuiDisambiguateAdapter가 구현한다.
type DisambiguateHandler interface {
	// RequestDisambiguate는 선택지를 표시하고 사용자의 선택을 기다린다.
	// 반환값: 선택된 인덱스 (0=첫번째, 1=두번째, ..., -1=취소/에러)
	RequestDisambiguate(question string, options []DisambiguateOption) (int, error)
}
