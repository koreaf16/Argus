// Package tui
// File: query_adapter.go
// Description: query.QueryEventSink 를 TUI bubbletea 메시지로 변환하는 어댑터
// Responsibility: query.Engine 이벤트 → tea.Msg 변환 (EventHandler 의존 제거)

package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/yourorg/infractl/internal/agent/query"
)

// TUIQueryEventSink 는 query.QueryEventSink 를 구현하여 query.Engine 이벤트를
// bubbletea TUI 메시지로 변환한다. B.10 이후 EventHandler 를 완전히 대체한다.
type TUIQueryEventSink struct {
	program *tea.Program
}

// NewTUIQueryEventSink 는 TUIQueryEventSink 를 생성한다.
// tea.Program 은 SetProgram 으로 나중에 주입할 수 있다.
func NewTUIQueryEventSink() *TUIQueryEventSink {
	return &TUIQueryEventSink{}
}

// SetProgram 은 이벤트 전달에 사용할 bubbletea 프로그램을 주입한다.
func (s *TUIQueryEventSink) SetProgram(p *tea.Program) {
	s.program = p
}

func (s *TUIQueryEventSink) send(msg tea.Msg) {
	if s.program != nil {
		s.program.Send(msg)
	}
}

// HandleQueryEvent 는 query.QueryEvent 를 받아 적절한 TUI 메시지로 변환한다.
func (s *TUIQueryEventSink) HandleQueryEvent(ev query.QueryEvent) {
	switch e := ev.(type) {
	case query.EventStreamStart:
		s.send(ThinkingStartMsg{Tier: e.Tier, Model: e.Model})

	case query.EventAssistantChunk:
		if e.Thinking {
			s.send(ThinkingTokenMsg(e.Text))
		} else {
			s.send(TokenMsg(e.Text))
		}

	case query.EventError:
		s.send(ErrorMsg{Err: e.Err})

	case query.EventTerminal:
		// ResponseDoneMsg 는 consumeEngineEvents 가 방출 — 여기서는 no-op.

	case query.EventToolUseStart:
		// 중복 방지: handler.OnToolStart(tool_exec.go:241)가 이미 ToolStartMsg를 TUI로 발사.

	case query.EventToolResult:
		// 중복 방지: handler.OnToolEnd(tool_exec.go:351,406,422)가 이미 ToolEndMsg를 TUI로 발사.

	case query.EventAssistantResponse:
		// 히스토리 재구성용 — UI 에는 별도 처리 불필요.
	}
}
