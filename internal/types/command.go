// Package types — REPL 슬래시 커맨드 타입 정의.
//
// 파일 역할: /help, /model, /exit 등 REPL 슬래시 커맨드의 기본 타입을 정의한다.
//
//	각 커맨드는 이름·설명·실행 함수를 가진다.
//	Phase 2에서 PromptCommand, LocalJSXCommand 등을 추가할 예정이다.
//
// 포함 모듈:
//   - CommandBase: 모든 커맨드 공통 필드.
//   - LocalCommandResult: 로컬 커맨드 실행 결과.
//   - CommandResultDisplay: 결과 표시 방식.
//
// 호출/사용 방식:
//   - internal/repl/commands/ 에서 각 커맨드 구현 시 참조.
//
// 연결:
//   - 원본: src/types/command.ts (Phase 1 핵심만 포함)
package types

// CommandResultDisplay 는 커맨드 결과 표시 방식이다.
type CommandResultDisplay string

const (
	CommandDisplaySkip   CommandResultDisplay = "skip"
	CommandDisplaySystem CommandResultDisplay = "system"
	CommandDisplayUser   CommandResultDisplay = "user"
)

// LocalCommandResult 는 로컬 커맨드 실행 결과다.
type LocalCommandResult struct {
	Type  string `json:"type"` // "text" | "skip"
	Value string `json:"value,omitempty"`
}

// CommandBase 는 모든 슬래시 커맨드에 공통으로 들어가는 필드다.
type CommandBase struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Aliases      []string `json:"aliases,omitempty"`
	IsHidden     bool     `json:"isHidden,omitempty"`
	ArgumentHint string   `json:"argumentHint,omitempty"`
	// IsEnabled 가 nil 이면 항상 활성.
	IsEnabled func() bool `json:"-"`
}

// GetCommandName 은 커맨드의 표시 이름을 반환한다 (Name 과 동일).
func GetCommandName(cmd CommandBase) string {
	return cmd.Name
}

// IsCommandEnabled 는 커맨드 활성 여부를 반환한다. IsEnabled 가 nil 이면 true.
func IsCommandEnabled(cmd CommandBase) bool {
	if cmd.IsEnabled == nil {
		return true
	}
	return cmd.IsEnabled()
}
