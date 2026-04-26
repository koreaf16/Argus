// Package lifecycle
// File: manager.go
// Description: 라이프사이클 훅 생명주기 관리자 — 등록, 발화, 활성화/비활성화, 삭제
// Responsibility: 이벤트 발생 시 해당 이벤트의 활성 훅을 조회하고 스크립트를 실행

package lifecycle

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/yourorg/infractl/internal/store"
)

// Manager는 라이프사이클 훅 생명주기를 관리한다.
type Manager struct {
	store store.HookStore
}

// NewManager는 Manager를 생성한다.
func NewManager(s store.HookStore) *Manager {
	return &Manager{store: s}
}

// Register는 새 훅을 등록하고 ID를 반환한다.
func (m *Manager) Register(ctx context.Context, event Event, condition, scriptPath string) (int64, error) {
	id, err := m.store.SaveHook(ctx, store.Hook{
		Event:      string(event),
		Condition:  condition,
		ScriptPath: scriptPath,
		Enabled:    true,
	})
	if err != nil {
		return 0, fmt.Errorf("register hook: %w", err)
	}
	slog.Info("lifecycle hook registered", "id", id, "event", event, "script", scriptPath)
	return id, nil
}

// List는 모든 훅을 반환한다.
func (m *Manager) List(ctx context.Context) ([]store.Hook, error) {
	return m.store.ListHooks(ctx)
}

// SetEnabled는 훅 활성/비활성 상태를 변경한다.
func (m *Manager) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	return m.store.SetHookEnabled(ctx, id, enabled)
}

// Delete는 훅을 삭제한다.
func (m *Manager) Delete(ctx context.Context, id int64) error {
	return m.store.DeleteHook(ctx, id)
}

// Fire는 이벤트를 발생시켜 매칭되는 활성 훅을 순차 실행한다.
// 훅 실행 실패는 WARN으로 기록하고 계속 진행한다.
func (m *Manager) Fire(ctx context.Context, hctx HookContext) {
	hooks, err := m.store.ListHooksByEvent(ctx, string(hctx.Event))
	if err != nil {
		slog.Warn("list hooks by event", "event", hctx.Event, "err", err)
		return
	}
	for _, h := range hooks {
		if !Matches(h, hctx) {
			continue
		}
		if err := m.runScript(ctx, h, hctx); err != nil {
			slog.Warn("lifecycle hook script failed", "id", h.ID, "script", h.ScriptPath, "err", err)
		}
	}
}

// runScript는 훅 스크립트를 실행한다.
// 컨텍스트 정보를 환경변수로 전달한다.
func (m *Manager) runScript(ctx context.Context, h store.Hook, hctx HookContext) error {
	if _, err := os.Stat(h.ScriptPath); err != nil {
		return fmt.Errorf("script not found: %s", h.ScriptPath)
	}

	cmd := exec.CommandContext(ctx, h.ScriptPath)
	cmd.Env = append(os.Environ(),
		"HOOK_EVENT="+string(hctx.Event),
		"HOOK_TOOL="+hctx.ToolName,
		"HOOK_SERVER="+hctx.Server,
		"HOOK_OUTPUT="+hctx.Output,
		"HOOK_ERROR="+hctx.Error,
		"HOOK_SERVICE="+hctx.ServiceType,
	)
	for k, v := range hctx.Args {
		cmd.Env = append(cmd.Env, "HOOK_ARG_"+strings.ToUpper(k)+"="+v)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("exit: %w, output: %s", err, strings.TrimSpace(string(out)))
	}
	slog.Debug("lifecycle hook executed", "id", h.ID, "event", hctx.Event, "output", strings.TrimSpace(string(out)))
	return nil
}
