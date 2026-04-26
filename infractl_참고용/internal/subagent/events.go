// Package subagent
// File: events.go
// Description: 서브에이전트 실시간 실행 이벤트 타입 및 콜백 정의
// Responsibility: 서브에이전트 진행 상태를 TUI로 스트리밍하기 위한 이벤트 계약 제공

package subagent

import "time"

// EventType은 서브에이전트 실행 중 발생하는 이벤트 종류이다.
type EventType string

const (
	// EventAgentStart는 에이전트 실행이 시작될 때 발생한다.
	EventAgentStart EventType = "agent_start"
	// EventToolCall은 에이전트가 도구를 호출하기 직전에 발생한다.
	EventToolCall EventType = "tool_call"
	// EventAgentDone은 에이전트 실행이 완료되었을 때 발생한다.
	EventAgentDone EventType = "agent_done"
)

// Event는 서브에이전트 실행 중 발생하는 단일 이벤트이다.
type Event struct {
	Type      EventType
	AgentType AgentType
	Server    string

	// EventToolCall 전용
	ToolName string
	ToolArg  string // 표시용 간략 인자

	// EventAgentDone 전용
	ToolUses     int
	InputTokens  int
	OutputTokens int
	Duration     time.Duration
	Success      bool
}

// EventCallback은 서브에이전트 이벤트를 수신하는 콜백 함수 타입이다.
// goroutine-safe하게 구현되어야 한다.
type EventCallback func(Event)
