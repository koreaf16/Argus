package workspace

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/koreaf16/argus/internal/services/inventory"
)

// globalInventoryRunner is shared across all Manager instances (process-wide singleton).
var globalInventoryRunner = inventory.NewRunner()

// makeInventoryExec returns an ExecFunc that runs scripts via the inventory
// SSH channel (fresh session, does not touch the exec PTY lane).
func (m *Manager) makeInventoryExec(alias string) inventory.ExecFunc {
	return func(ctx context.Context, script string) (string, error) {
		_ = m.channelPool() // ensure pool is initialized
		ch, err := m.ChannelManager().AcquireInventory(ctx, alias)
		if err != nil {
			return "", fmt.Errorf("inventory exec: %w", err)
		}
		return ch.Run(ctx, script)
	}
}

// makeLocalInventoryExec returns an ExecFunc that runs bash scripts locally.
// Returns nil when bash is not available (Windows without bash).
func (m *Manager) makeLocalInventoryExec() inventory.ExecFunc {
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("bash"); err != nil {
			return nil
		}
	}
	return func(ctx context.Context, script string) (string, error) {
		res, err := m.execLocal(ctx, script, ExecOptions{Shell: "bash"})
		if err != nil {
			return "", err
		}
		return res.Stdout, nil
	}
}

// StartInventoryScan begins a background inventory scan for alias. If a scan
// is already in progress the call is a no-op. onDone is called with the
// result from the background goroutine.
func (m *Manager) StartInventoryScan(
	ctx context.Context,
	alias string,
	onDone func(inventory.InventorySnapshot),
) {
	resolved := m.ResolveAlias(alias)
	if resolved == LocalAlias {
		return
	}
	execFn := m.makeInventoryExec(resolved)
	globalInventoryRunner.CollectAsync(ctx, resolved, execFn, onDone)
}

// GetInventorySnapshot returns the cached inventory for alias, if available.
func (m *Manager) GetInventorySnapshot(alias string) (inventory.InventorySnapshot, bool) {
	return globalInventoryRunner.Cache().Get(m.ResolveAlias(alias))
}

// RescanInventory forces a fresh inventory scan and waits for completion.
// Deprecated: prefer RescanInventoryStreaming for streaming phase callbacks.
func (m *Manager) RescanInventory(ctx context.Context, alias string) (inventory.InventorySnapshot, error) {
	resolved := m.ResolveAlias(alias)
	if resolved == LocalAlias {
		return inventory.InventorySnapshot{}, fmt.Errorf("로컬 워크스페이스는 인벤토리 스캔을 지원하지 않습니다")
	}
	execFn := m.makeInventoryExec(resolved)
	return globalInventoryRunner.CollectSync(ctx, resolved, execFn, true)
}

// RescanInventoryStreaming runs a streaming inventory scan (header → ready) calling onPhase at each phase.
// For LocalAlias on Windows without bash it immediately returns a disabled snapshot.
// This call blocks until the scan completes.
func (m *Manager) RescanInventoryStreaming(ctx context.Context, alias string, onPhase inventory.OnPhaseFunc) {
	if onPhase == nil {
		onPhase = func(inventory.InventoryPhase, inventory.InventorySnapshot, error) {}
	}

	resolved := m.ResolveAlias(alias)

	if resolved == LocalAlias {
		execFn := m.makeLocalInventoryExec()
		if execFn == nil {
			disabled := inventory.InventorySnapshot{
				Alias:       LocalAlias,
				Status:      inventory.StatusDisabled,
				CollectedAt: time.Now(),
			}
			globalInventoryRunner.Cache().Set(LocalAlias, disabled)
			onPhase(inventory.PhaseReady, disabled, nil)
			return
		}
		globalInventoryRunner.CollectStreaming(ctx, LocalAlias, execFn, onPhase)
		return
	}

	execFn := m.makeInventoryExec(resolved)
	globalInventoryRunner.CollectStreaming(ctx, resolved, execFn, onPhase)
}

// CollectLocalSystemHeader runs only the system_header probe for the local workspace
// and caches the partial result. Used at startup to ensure system info is available
// before the first user message.
func (m *Manager) CollectLocalSystemHeader(ctx context.Context) {
	execFn := m.makeLocalInventoryExec()
	if execFn == nil {
		return
	}
	tctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	globalInventoryRunner.CollectHeader(tctx, LocalAlias, execFn)
}

// InvalidateInventory removes the cached inventory for alias (call on disconnect).
func (m *Manager) InvalidateInventory(alias string) {
	globalInventoryRunner.Cache().Invalidate(m.ResolveAlias(alias))
}
