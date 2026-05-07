package workspace

import (
	"context"
	"fmt"

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
func (m *Manager) RescanInventory(ctx context.Context, alias string) (inventory.InventorySnapshot, error) {
	resolved := m.ResolveAlias(alias)
	if resolved == LocalAlias {
		return inventory.InventorySnapshot{}, fmt.Errorf("로컬 워크스페이스는 인벤토리 스캔을 지원하지 않습니다")
	}
	execFn := m.makeInventoryExec(resolved)
	return globalInventoryRunner.CollectSync(ctx, resolved, execFn, true)
}

// InvalidateInventory removes the cached inventory for alias (call on disconnect).
func (m *Manager) InvalidateInventory(alias string) {
	globalInventoryRunner.Cache().Invalidate(m.ResolveAlias(alias))
}
