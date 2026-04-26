// Package types — 훅 이벤트·결과 타입.
//
// 파일 역할: 도구 사용 전후, 세션 시작 등 다양한 시점에 실행되는
//
//	훅 시스템의 기본 타입을 정의한다.
//	Phase 1 에서는 핵심 이벤트 이름과 결과 타입만 포함한다.
//
// 포함 모듈:
//   - HookEvent: 훅이 발생하는 이벤트 이름 열거형.
//   - HookResult: 훅 실행 결과.
//   - HookProgress: 훅 진행 상태.
//   - IsHookEvent(): 이벤트 이름 유효성 검사.
//
// 호출/사용 방식:
//   - internal/query/engine.go 에서 PreToolUse / PostToolUse 훅 호출 시 참조.
//
// 연결:
//   - 원본: src/types/hooks.ts (Phase 1 핵심만 포함)
package types

// HookEvent 는 훅이 발생하는 이벤트 이름이다.
type HookEvent string

const (
	HookEventPreToolUse         HookEvent = "PreToolUse"
	HookEventPostToolUse        HookEvent = "PostToolUse"
	HookEventPostToolUseFailure HookEvent = "PostToolUseFailure"
	HookEventSessionStart       HookEvent = "SessionStart"
	HookEventStop               HookEvent = "Stop"
	HookEventUserPrompt         HookEvent = "UserPromptSubmit"
	HookEventNotification       HookEvent = "Notification"
	HookEventPermDenied         HookEvent = "PermissionDenied"
	HookEventPermRequest        HookEvent = "PermissionRequest"
	HookEventSetup              HookEvent = "Setup"
	HookEventSubagentStart      HookEvent = "SubagentStart"
	HookEventFileChanged        HookEvent = "FileChanged"
	HookEventCwdChanged         HookEvent = "CwdChanged"
)

// AllHookEvents 는 유효한 훅 이벤트 이름 집합이다.
var AllHookEvents = map[HookEvent]bool{
	HookEventPreToolUse:         true,
	HookEventPostToolUse:        true,
	HookEventPostToolUseFailure: true,
	HookEventSessionStart:       true,
	HookEventStop:               true,
	HookEventUserPrompt:         true,
	HookEventNotification:       true,
	HookEventPermDenied:         true,
	HookEventPermRequest:        true,
	HookEventSetup:              true,
	HookEventSubagentStart:      true,
	HookEventFileChanged:        true,
	HookEventCwdChanged:         true,
}

// IsHookEvent 는 문자열이 유효한 HookEvent 인지 확인한다.
func IsHookEvent(value string) bool {
	return AllHookEvents[HookEvent(value)]
}

// HookOutcome 은 훅 실행 결과 상태다.
type HookOutcome string

const (
	HookOutcomeSuccess          HookOutcome = "success"
	HookOutcomeBlocking         HookOutcome = "blocking"
	HookOutcomeNonBlockingError HookOutcome = "non_blocking_error"
	HookOutcomeCancelled        HookOutcome = "cancelled"
)

// HookBlockingError 는 훅이 실행을 차단할 때 반환하는 오류 정보다.
type HookBlockingError struct {
	BlockingError string `json:"blockingError"`
	Command       string `json:"command"`
}

// HookResult 는 단일 훅 실행 결과다.
type HookResult struct {
	Outcome             HookOutcome        `json:"outcome"`
	Message             *Message           `json:"message,omitempty"`
	BlockingError       *HookBlockingError `json:"blockingError,omitempty"`
	PreventContinuation bool               `json:"preventContinuation,omitempty"`
	StopReason          string             `json:"stopReason,omitempty"`
	PermissionBehavior  PermissionBehavior `json:"permissionBehavior,omitempty"`
	AdditionalContext   string             `json:"additionalContext,omitempty"`
	UpdatedInput        map[string]any     `json:"updatedInput,omitempty"`
}

// HookProgress 는 훅 진행 상태 알림이다.
type HookProgress struct {
	Type          string    `json:"type"` // "hook_progress"
	HookEvent     HookEvent `json:"hookEvent"`
	HookName      string    `json:"hookName"`
	Command       string    `json:"command"`
	StatusMessage string    `json:"statusMessage,omitempty"`
}
