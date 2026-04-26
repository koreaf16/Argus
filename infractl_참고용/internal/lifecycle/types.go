// Package lifecycle
// File: types.go
// Description: 라이프사이클 훅 이벤트 상수 및 실행 컨텍스트 타입 정의
// Responsibility: 훅 이벤트 식별자와 스크립트에 전달되는 컨텍스트 구조체 제공

package lifecycle

// Event는 훅 이벤트 식별자이다.
type Event string

const (
	EventBeforeExecute  Event = "before_execute"
	EventAfterExecute   Event = "after_execute"
	EventOnError        Event = "on_error"
	EventOnConnect      Event = "on_connect"
	EventOnAlert        Event = "on_alert"
	EventOnSessionStart Event = "on_session_start"
)

// HookContext는 훅 스크립트에 전달되는 실행 컨텍스트이다.
type HookContext struct {
	Event       Event
	ToolName    string            // 실행된 도구명 (before/after_execute, on_error)
	Server      string            // 대상 서버명
	Args        map[string]string // 도구 인자 (string으로 변환)
	Output      string            // 도구 결과 (after_execute)
	Error       string            // 에러 메시지 (on_error)
	ServiceType string            // 서비스 타입 (on_connect)
}
