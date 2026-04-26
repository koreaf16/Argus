// Package planmode
// File: transition.go
// Description: Plan Mode 진입/종료 전환 로직
// Responsibility: Enter/Exit 시 상태 전환, 대기열 정리

// Ported from: claude_cli/src/tools/EnterPlanModeTool/EnterPlanModeTool.ts:77-102 (Enter)
//              claude_cli/src/tools/ExitPlanModeTool/ExitPlanModeV2Tool.ts:243-403 (Exit)
// 핵심 패턴: mode 전환 → system prompt 주입은 호출자(loop.go) 책임,
//            State 는 모드/큐만 관리한다.

package planmode

import "log/slog"

// Enter 는 Plan Mode 를 활성화한다.
// 이미 활성화된 경우 no-op 이다.
func (s *State) Enter() {
	if s.IsActive() {
		return
	}
	s.setMode(Active)
	slog.Info("plan mode entered")
}

// Exit 는 Plan Mode 를 종료하고 대기 중인 명령 목록을 반환한다.
// 활성화 상태가 아닌 경우 nil 을 반환하고 no-op 이다.
// 반환된 []PendingCommand 의 실행/폐기는 approval.go 의 책임이다.
func (s *State) Exit() []PendingCommand {
	if !s.IsActive() {
		return nil
	}
	s.setMode(Off)
	cmds := s.queue.Drain()
	slog.Info("plan mode exited", "pending_commands", len(cmds))
	return cmds
}

// Toggle 은 Plan Mode 를 토글하고 변경 후 활성 여부를 반환한다.
// Exit 경로에서 대기열을 drain 하므로 TUI 단축키용으로 적합하지 않을 수 있다.
// 대기열 항목이 있을 때는 approval.TriggerIfNeeded 를 먼저 호출하라.
func (s *State) Toggle() bool {
	if s.IsActive() {
		s.Exit()
		return false
	}
	s.Enter()
	return true
}
