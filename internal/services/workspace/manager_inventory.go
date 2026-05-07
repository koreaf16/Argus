package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
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

func disabledLocalInventorySnapshot() inventory.InventorySnapshot {
	return inventory.InventorySnapshot{
		Alias:       LocalAlias,
		Status:      inventory.StatusDisabled,
		CollectedAt: time.Now(),
		Errors: map[string]string{
			"local_inventory": "local inventory requires bash; bash was not found on this Windows host",
		},
	}
}

func nativeLocalInventorySnapshot(ctx context.Context, status inventory.Status) inventory.InventorySnapshot {
	started := time.Now()
	sys := &inventory.SystemInfo{
		OS:       runtime.GOOS + "/" + runtime.GOARCH,
		OSPretty: strings.TrimSpace(os.Getenv("OS")),
		User:     currentLocalUser(),
		Shell:    localShellLabel(),
	}
	if cwd, err := os.Getwd(); err == nil {
		sys.CWD = cwd
	}
	if runtime.GOOS == "windows" {
		populateWindowsNativeSystemInfo(ctx, sys)
	}
	return inventory.InventorySnapshot{
		Alias:       LocalAlias,
		Status:      status,
		CollectedAt: started,
		DurationMs:  time.Since(started).Milliseconds(),
		System:      sys,
	}
}

func currentLocalUser() string {
	for _, key := range []string{"USERNAME", "USER"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func localShellLabel() string {
	if runtime.GOOS == "windows" {
		if strings.TrimSpace(os.Getenv("PSModulePath")) != "" {
			return "powershell"
		}
		if comspec := strings.TrimSpace(os.Getenv("ComSpec")); comspec != "" {
			return comspec
		}
		return "windows"
	}
	if shell := strings.TrimSpace(os.Getenv("SHELL")); shell != "" {
		return shell
	}
	return "sh"
}

func populateWindowsNativeSystemInfo(parent context.Context, sys *inventory.SystemInfo) {
	ctx := parent
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if value := runLocalPowerShellLine(ctx, "$os=Get-CimInstance Win32_OperatingSystem; ($os.Caption + ' ' + $os.Version).Trim()"); value != "" {
		sys.OSPretty = value
	}
	if value := runLocalPowerShellLine(ctx, "$os=Get-CimInstance Win32_OperatingSystem; 'total={0:N1}GiB free={1:N1}GiB' -f ($os.TotalVisibleMemorySize/1MB),($os.FreePhysicalMemory/1MB)"); value != "" {
		sys.Memory = value
	}
	if value := runLocalPowerShellLine(ctx, "$d=Get-CimInstance Win32_LogicalDisk -Filter \"DeviceID='$env:SystemDrive'\"; if($d){ 'drive={0} size={1:N1}GiB free={2:N1}GiB' -f $d.DeviceID,($d.Size/1GB),($d.FreeSpace/1GB) }"); value != "" {
		sys.Disk = value
	}
	if value := runLocalPowerShellLine(ctx, "(Get-Service | Where-Object {$_.Status -eq 'Running'} | Measure-Object).Count"); value != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			sys.ServiceCount = n
		}
	}
	if value := runLocalPowerShellLine(ctx, "(Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue | Measure-Object).Count"); value != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			sys.ListenerCount = n
		}
	}
}

func runLocalPowerShellLine(ctx context.Context, script string) string {
	exe, err := exec.LookPath("powershell")
	if err != nil {
		exe, err = exec.LookPath("pwsh")
	}
	if err != nil {
		return ""
	}
	out, err := exec.CommandContext(ctx, exe, "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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

// SetInventorySnapshot seeds the inventory cache for alias. Used in tests and
// for pre-populating known system state without running a full scan.
func (m *Manager) SetInventorySnapshot(alias string, snap inventory.InventorySnapshot) {
	globalInventoryRunner.Cache().Set(m.ResolveAlias(alias), snap)
}

// RescanInventory forces a fresh inventory scan and waits for completion.
// Deprecated: prefer RescanInventoryStreaming for streaming phase callbacks.
func (m *Manager) RescanInventory(ctx context.Context, alias string) (inventory.InventorySnapshot, error) {
	resolved := m.ResolveAlias(alias)
	if resolved == LocalAlias {
		execFn := m.makeLocalInventoryExec()
		if execFn == nil {
			snap := nativeLocalInventorySnapshot(ctx, inventory.StatusReady)
			globalInventoryRunner.Cache().Set(LocalAlias, snap)
			return snap, nil
		}
		return globalInventoryRunner.CollectSync(ctx, LocalAlias, execFn, true)
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
			partial := nativeLocalInventorySnapshot(ctx, inventory.StatusPartial)
			globalInventoryRunner.Cache().Set(LocalAlias, partial)
			onPhase(inventory.PhaseHeader, partial, nil)
			ready := partial
			ready.Status = inventory.StatusReady
			ready.DurationMs = time.Since(partial.CollectedAt).Milliseconds()
			globalInventoryRunner.Cache().Set(LocalAlias, ready)
			onPhase(inventory.PhaseReady, ready, nil)
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
		tctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		globalInventoryRunner.Cache().Set(LocalAlias, nativeLocalInventorySnapshot(tctx, inventory.StatusReady))
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
