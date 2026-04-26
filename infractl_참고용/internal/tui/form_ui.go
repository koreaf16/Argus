// Package tui
// File: form_ui.go
// Description: 폼 입력 모드의 결과 타입 정의
// Responsibility: FormResult 타입 및 관련 상수 제공

package tui

// FormResult는 사용자가 폼을 완성하거나 취소했을 때의 결과다.
type FormResult struct {
	Values    map[string]string
	Cancelled bool
}
