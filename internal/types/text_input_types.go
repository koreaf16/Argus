// Package types — 텍스트 입력 컴포넌트 타입.
//
// 파일 역할: REPL 입력창(readline 기반)의 상태와 큐 타입을 정의한다.
//
//	Phase 1 에서는 QueuedCommand 와 QueuePriority 만 포함한다.
//	Phase 2 (Bubbletea TUI)에서 BaseTextInputProps 등이 추가된다.
//
// 포함 모듈:
//   - QueuePriority: 큐 우선순위 (now / next / later).
//   - QueuedCommand: 큐에 넣을 커맨드 타입.
//   - PromptInputMode: 입력 모드 열거형.
//
// 호출/사용 방식:
//   - internal/repl/repl.go 에서 커맨드 큐 관리 시 참조.
//
// 연결:
//   - 원본: src/types/textInputTypes.ts (Phase 1 핵심만 포함)
package types

// QueuePriority 는 커맨드 큐의 처리 우선순위다.
type QueuePriority string

const (
	QueuePriorityNow   QueuePriority = "now"
	QueuePriorityNext  QueuePriority = "next"
	QueuePriorityLater QueuePriority = "later"
)

// PromptInputMode 는 입력창의 현재 모드다.
type PromptInputMode string

const (
	InputModeBash               PromptInputMode = "bash"
	InputModePrompt             PromptInputMode = "prompt"
	InputModeOrphanedPermission PromptInputMode = "orphaned-permission"
	InputModeTaskNotification   PromptInputMode = "task-notification"
)

// QueuedCommand 는 처리 대기 중인 사용자 커맨드다.
type QueuedCommand struct {
	Value    string          `json:"value"`
	Mode     PromptInputMode `json:"mode"`
	Priority QueuePriority   `json:"priority,omitempty"`
	UUID     string          `json:"uuid,omitempty"`
	IsMeta   bool            `json:"isMeta,omitempty"`
	Origin   MessageOrigin   `json:"origin,omitempty"`
}
