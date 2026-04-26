// Package agent
// File: session.go
// Description: 대화 세션 저장/복원 로직
// Responsibility: 히스토리 ↔ SQLite 세션 변환, 세션 생성/복원

package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/store"
)

// NewSession은 새 대화 세션을 생성하고 ID를 설정한다.
// title이 비어 있으면 "새 대화"로 초기화하며, 이후 첫 메시지로 업데이트된다.
func (a *Agent) NewSession(ctx context.Context) (int64, error) {
	if a.sessionStore == nil {
		return 0, nil
	}
	id, err := a.sessionStore.CreateConversation(ctx, "새 대화")
	if err != nil {
		return 0, fmt.Errorf("create session: %w", err)
	}
	a.currentSessionID = id
	slog.Debug("new session created", "id", id)

	a.auditCloseRun(ctx)
	a.auditRunID = a.auditStartRun(ctx)
	a.auditTitleSet = false

	return id, nil
}

// UpdateSessionTitle은 첫 번째 사용자 입력으로 세션 제목을 업데이트한다.
func (a *Agent) UpdateSessionTitle(ctx context.Context, title string) {
	if a.sessionStore == nil || a.currentSessionID == 0 {
		return
	}
	if len(title) > 50 {
		title = title[:50] + "..."
	}
	if err := a.sessionStore.UpdateConversationTitle(ctx, a.currentSessionID, title); err != nil {
		slog.Warn("update session title", "err", err)
	}
}

// RestoreSession은 세션 ID의 메시지를 로드하여 히스토리를 복원한다.
func (a *Agent) RestoreSession(ctx context.Context, sessionID int64) error {
	if a.sessionStore == nil {
		return fmt.Errorf("session store not available")
	}

	msgs, err := a.sessionStore.LoadMessages(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("load session messages: %w", err)
	}

	a.history = make([]llm.Message, 0, len(msgs))
	for _, sm := range msgs {
		// system 메시지는 히스토리에서 제외 (매 턴 재생성)
		if sm.Role == "system" {
			continue
		}
		a.history = append(a.history, llmMessageFromSession(sm))
	}

	// 복원 후 즉시 히스토리 정리 (maxHistory 준수)
	a.trimHistory()

	a.currentSessionID = sessionID
	slog.Info("session restored", "id", sessionID, "messages", len(a.history))
	return nil
}

// GetHistory는 현재 히스토리를 반환한다.
func (a *Agent) GetHistory() []llm.Message {
	return a.history
}

// CurrentSessionID는 현재 대화 세션 ID를 반환한다.
func (a *Agent) CurrentSessionID() int64 {
	return a.currentSessionID
}

// saveToolResults는 tool 역할 메시지들을 세션에 저장한다.
func (a *Agent) saveToolResults(ctx context.Context, msgs []store.SessionMessage) {
	if a.sessionStore == nil || a.currentSessionID == 0 {
		return
	}
	for _, sm := range msgs {
		if _, err := a.sessionStore.SaveMessage(ctx, sm); err != nil {
			slog.Warn("save tool result message", "err", err)
		}
	}
}
