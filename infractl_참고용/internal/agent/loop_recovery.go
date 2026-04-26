// Package agent
// File: loop_recovery.go
// Description: 세션 메시지 저장 헬퍼
// Responsibility: saveSessionMessage — 세션 스토어 저장 및 RAG 인덱싱

package agent

import (
	"context"
	"log/slog"

	"github.com/yourorg/infractl/internal/llm"
)

// saveSessionMessage는 메시지를 세션 스토어에 저장하고 RAG 인덱싱한다.
func (a *Agent) saveSessionMessage(ctx context.Context, msg llm.Message) {
	if a.sessionStore == nil || a.currentSessionID <= 0 {
		return
	}
	sm := sessionMessageFromLLM(msg, a.currentSessionID)
	id, err := a.sessionStore.SaveMessage(ctx, sm)
	if err != nil {
		slog.Warn("save session message", "role", msg.Role, "err", err)
		return
	}
	if a.ragManager != nil {
		sm.ID = id
		if idxErr := a.ragManager.IndexSessionMessage(ctx, sm); idxErr != nil {
			slog.Warn("index session message", "id", id, "err", idxErr)
		}
	}
}
