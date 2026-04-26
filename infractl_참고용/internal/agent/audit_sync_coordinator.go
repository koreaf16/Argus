// Package agent
// File: audit_sync_coordinator.go
// Description: 감사 동기화 코디네이터 — Agent의 감사 이벤트를 원격 DB에 동기화
// Responsibility: AuditRemoteTransport 어댑터 + push SQL 생성 + pull/retry 트리거

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/store"
)

// sshAuditTransport는 executor.Executor를 store.AuditRemoteTransport로 어댑트한다.
type sshAuditTransport struct {
	exec   executor.Executor
	ftExec executor.FileTransferExecutor
}

// DownloadFile은 SFTP로 원격 파일을 로컬에 내려받는다.
func (t *sshAuditTransport) DownloadFile(ctx context.Context, remotePath, localPath string) error {
	if t.ftExec == nil {
		return fmt.Errorf("no file transfer executor available for audit pull")
	}
	return t.ftExec.Download(ctx, remotePath, localPath, nil)
}

// RunSQL은 원격 sqlite3 CLI를 통해 단일 SQL 문을 실행한다.
func (t *sshAuditTransport) RunSQL(ctx context.Context, dbPath, sql string) error {
	cmd := fmt.Sprintf("sqlite3 %s %s", store.QuoteSQL(dbPath), store.QuoteSQL(sql))
	result, err := t.exec.Execute(ctx, cmd)
	if err != nil {
		return fmt.Errorf("remote sqlite3 exec: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("sqlite3 exit %d: %s", result.ExitCode, result.Stderr)
	}
	return nil
}

// auditSyncCoordinator는 감사 이벤트 발생 후 원격 동기화를 조율한다.
type auditSyncCoordinator struct {
	engine       *store.SyncEngine
	transport    store.AuditRemoteTransport
	serverName   string
	remoteDBPath string
}

// newAuditSyncCoordinator는 서버 attach 시점에 coordinator를 생성한다.
func newAuditSyncCoordinator(engine *store.SyncEngine, exec executor.Executor, serverName, remoteDBPath string) *auditSyncCoordinator {
	var ft executor.FileTransferExecutor
	if fte, ok := exec.(executor.FileTransferExecutor); ok {
		ft = fte
	}
	return &auditSyncCoordinator{
		engine:       engine,
		transport:    &sshAuditTransport{exec: exec, ftExec: ft},
		serverName:   serverName,
		remoteDBPath: remoteDBPath,
	}
}

// pullOnAttach는 서버 attach 직후 원격 audit.db를 로컬에 병합한다.
func (c *auditSyncCoordinator) pullOnAttach(ctx context.Context) {
	if err := c.engine.PullOnAttach(ctx, c.transport, c.remoteDBPath, c.serverName); err != nil {
		slog.Warn("audit pull on attach", "server", c.serverName, "err", err)
	}
}

// pushRun은 audit_runs 행을 원격에 푸시한다.
func (c *auditSyncCoordinator) pushRun(ctx context.Context, run store.AuditRun) {
	sql := fmt.Sprintf(
		"INSERT OR IGNORE INTO audit_runs(id,workspace_id,server_name,kind,title,user_prompt,active_plan_id,status,started_at,ended_at,summary_md,metadata_json) VALUES(%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)",
		store.QuoteSQL(run.ID),
		store.QuoteSQL(run.ServerID),
		store.QuoteSQL(run.ServerName),
		store.QuoteSQL(string(run.Kind)),
		store.QuoteSQL(run.Title),
		store.QuoteSQL(run.UserPrompt),
		sqlNullStr(run.ActivePlanID),
		store.QuoteSQL(string(run.Status)),
		store.QuoteSQL(run.StartedAt.UTC().Format(time.RFC3339)),
		sqlNullTime(run.EndedAt),
		store.QuoteSQL(run.SummaryMD),
		store.QuoteSQL(run.MetadataJSON),
	)
	go c.engine.PushSQL(ctx, c.transport, c.serverName, c.remoteDBPath, sql)
}

// pushStep은 audit_steps 행을 원격에 푸시한다.
func (c *auditSyncCoordinator) pushStep(ctx context.Context, step store.AuditStep) {
	sql := fmt.Sprintf(
		"INSERT OR IGNORE INTO audit_steps(id,run_id,planned_step_id,seq,label,exec_log_id,status,result_md,deviation_md,started_at,ended_at) VALUES(%s,%s,%s,%d,%s,%s,%s,%s,%s,%s,%s)",
		store.QuoteSQL(step.ID),
		store.QuoteSQL(step.RunID),
		sqlNullStr(step.PlannedStepID),
		step.Seq,
		store.QuoteSQL(step.Label),
		sqlNullInt64(step.ExecLogID),
		store.QuoteSQL(string(step.Status)),
		store.QuoteSQL(step.ResultMD),
		store.QuoteSQL(step.DeviationMD),
		store.QuoteSQL(step.StartedAt.UTC().Format(time.RFC3339)),
		sqlNullTime(step.EndedAt),
	)
	go c.engine.PushSQL(ctx, c.transport, c.serverName, c.remoteDBPath, sql)
}

// retryOutbox는 대기 중인 outbox 항목을 재시도한다.
func (c *auditSyncCoordinator) retryOutbox(ctx context.Context) {
	c.engine.RetryOutbox(ctx, c.transport, c.serverName, c.remoteDBPath)
}

func sqlNullStr(s string) string {
	if s == "" {
		return "NULL"
	}
	return store.QuoteSQL(s)
}

func sqlNullInt64(v *int64) string {
	if v == nil {
		return "NULL"
	}
	return fmt.Sprintf("%d", *v)
}

func sqlNullTime(t *time.Time) string {
	if t == nil {
		return "NULL"
	}
	return store.QuoteSQL(t.UTC().Format(time.RFC3339))
}

// remoteAuditDBPath는 서버의 워크스페이스 디렉토리 아래 audit.db 경로를 반환한다.
func remoteAuditDBPath(workspaceDir string) string {
	if workspaceDir == "" {
		return "~/audit.db"
	}
	return workspaceDir + "/audit.db"
}
