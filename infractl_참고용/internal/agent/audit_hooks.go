// Package agent
// File: audit_hooks.go
// Description: Agent에서 감사(Audit) 저장소로 이벤트를 전달하는 훅 레이어
// Responsibility: AuditStore 인터페이스 정의 및 Agent에서의 감사 이벤트 기록 헬퍼

package agent

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/yourorg/infractl/internal/store"
)

// AuditStore는 audit 레이어에 필요한 store 메서드의 하위 집합이다.
// *store.SQLiteStore 가 이를 충족하므로 별도 어댑터 없이 직접 주입된다.
type AuditStore interface {
	store.AuditRunStore
	store.AuditStepStore
	store.AuditQueryStore
	store.AuditPrivilegeStore
	store.AuditSyncStore
	store.AuditCheckStore
	store.AuditPlanStore
}

// auditRunID는 아직 생성되지 않은 상태를 나타내는 빈 문자열 상수이다.
const noAuditRun = ""

// auditStartRun은 새 audit run 을 생성하고 ID를 반환한다.
// 에러 발생 시 기존 흐름을 막지 않기 위해 warn 로그만 남기고 "" 을 반환한다.
func (a *Agent) auditStartRun(ctx context.Context) string {
	if a.auditStore == nil {
		return noAuditRun
	}
	serverName := "localhost"
	if a.activeServer != nil {
		serverName = a.activeServer.Name
	}
	id := uuid.New().String()
	run := store.AuditRun{
		ID:           id,
		ServerID:     serverName,
		ServerName:   serverName,
		Kind:         store.AuditRunKindAdHoc,
		Status:       store.AuditRunStatusPlanning,
		StartedAt:    time.Now().UTC(),
		MetadataJSON: "{}",
	}
	if err := a.auditStore.InsertAuditRun(ctx, run); err != nil {
		slog.Warn("audit: start run", "err", err)
		return noAuditRun
	}
	if a.auditSync != nil {
		a.auditSync.pushRun(ctx, run)
	}
	return id
}

// auditCloseRun은 이전 run 이 있으면 succeeded 로 마감한다.
// 새 세션 시작 직전에 호출한다.
func (a *Agent) auditCloseRun(ctx context.Context) {
	if a.auditStore == nil || a.auditRunID == noAuditRun {
		return
	}
	t := time.Now().UTC()
	if err := a.auditStore.UpdateAuditRunStatus(ctx, a.auditRunID, store.AuditRunStatusSucceeded, &t, ""); err != nil {
		slog.Warn("audit: close previous run", "err", err)
	}
	a.auditRunID = noAuditRun
}

// auditAttachPrompt는 첫 번째 사용자 프롬프트를 run 에 연결한다.
// 이미 title 이 있으면(두 번째 이상 turn) 스텝만 추가하고 title 은 건드리지 않는다.
func (a *Agent) auditAttachPrompt(ctx context.Context, userInput string) {
	if a.auditStore == nil || a.auditRunID == noAuditRun {
		return
	}
	if a.auditTitleSet {
		return
	}
	title := userInput
	if len(title) > 80 {
		title = title[:80] + "…"
	}
	if err := a.auditStore.UpdateAuditRunTitle(ctx, a.auditRunID, title, userInput); err != nil {
		slog.Warn("audit: attach prompt", "err", err)
		return
	}
	// status를 running 으로 전환 (planning → running)
	if err := a.auditStore.UpdateAuditRunStatus(ctx, a.auditRunID, store.AuditRunStatusRunning, nil, ""); err != nil {
		slog.Warn("audit: set run running", "err", err)
	}
	a.auditTitleSet = true
}

// auditAppendStep은 도구 실행 완료 후 step 을 audit_steps 에 추가한다.
func (a *Agent) auditAppendStep(ctx context.Context, execLogID int64, toolName string, success bool) {
	if a.auditStore == nil || a.auditRunID == noAuditRun {
		return
	}
	seq, err := a.auditStore.NextAuditStepSeq(ctx, a.auditRunID)
	if err != nil {
		slog.Warn("audit: next step seq", "err", err)
		return
	}
	status := store.AuditStepStatusOK
	if !success {
		status = store.AuditStepStatusFail
	}
	now := time.Now().UTC()
	step := store.AuditStep{
		ID:        uuid.New().String(),
		RunID:     a.auditRunID,
		Seq:       seq,
		Label:     toolName,
		ExecLogID: &execLogID,
		Status:    status,
		StartedAt: now,
		EndedAt:   &now,
	}
	if err := a.auditStore.InsertAuditStep(ctx, step); err != nil {
		slog.Warn("audit: append step", "tool", toolName, "err", err)
		return
	}
	if a.auditSync != nil {
		a.auditSync.pushStep(ctx, step)
	}
}

// SetAuditStore 는 AuditStore 를 주입한다.
func (a *Agent) SetAuditStore(s AuditStore) {
	a.auditStore = s
}

// SetAuditSyncEngine 는 SyncEngine 을 주입한다.
func (a *Agent) SetAuditSyncEngine(e *store.SyncEngine) {
	a.auditSyncEngine = e
}
