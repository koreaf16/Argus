// Package tui
// File: app_agent.go
// Description: AppModel의 에이전트 실행 관련 tea.Cmd 팩토리
// Responsibility: 에이전트 goroutine 실행과 세션 생성 커맨드 제공

package tui

import (
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"
)

// runAgent는 에이전트를 goroutine에서 실행하는 tea.Cmd를 반환한다.
func (m AppModel) runAgent(input string) tea.Cmd {
	ag := m.ag
	ctx := m.ctx
	reqID := m.reqID // 현재 세대 캡처 — 완료 시 stale 여부 판별에 사용
	return func() tea.Msg {
		_ = ag.Run(ctx, input)
		return AgentDoneMsg{ReqID: reqID}
	}
}

// createSessionCmd는 새 세션을 생성하는 tea.Cmd를 반환한다.
func (m AppModel) createSessionCmd(firstInput string) tea.Cmd {
	return func() tea.Msg {
		ctx := m.ctx
		id, err := m.ag.NewSession(ctx)
		if err != nil {
			slog.Warn("create session", "err", err)
			return nil
		}
		m.ag.UpdateSessionTitle(ctx, firstInput)
		_ = id
		return nil
	}
}
