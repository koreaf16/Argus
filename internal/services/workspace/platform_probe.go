package workspace

import (
	"context"
	"runtime"
	"strings"
	"time"

	"github.com/koreaf16/argus/internal/services/inventory"
	"github.com/koreaf16/argus/internal/utils"
)

const (
	PlatformUnknown = "unknown"
	PlatformWindows = "windows"
	PlatformUnix    = "unix"
)

func (m *Manager) DetectPlatform(ctx context.Context, alias string) (string, error) {
	alias = m.ResolveAlias(alias)
	if alias == LocalAlias {
		if runtime.GOOS == "windows" {
			return PlatformWindows, nil
		}
		return PlatformUnix, nil
	}

	if snap, ok := m.GetInventorySnapshot(alias); ok && snap.System != nil {
		if p := classifyPlatformText(snap.System.OS + "\n" + snap.System.Shell); p != PlatformUnknown {
			return p, nil
		}
	}

	session, err := m.ensureSession(alias)
	if err != nil {
		return PlatformUnknown, err
	}
	platform, evidence, err := session.detectPlatform(ctx)
	if err != nil {
		return PlatformUnknown, err
	}
	if platform != PlatformUnknown {
		m.mergePlatformToInventory(alias, platform, evidence)
	}
	return platform, nil
}

func (m *Manager) DetectBashAvailability(ctx context.Context, alias string) (bool, error) {
	alias = m.ResolveAlias(alias)
	if alias == LocalAlias {
		if runtime.GOOS != "windows" {
			return true, nil
		}
		sh, err := utils.FindSuitableShell()
		if err != nil {
			return false, nil
		}
		lower := strings.ToLower(strings.TrimSpace(sh))
		return strings.Contains(lower, "bash"), nil
	}

	platform, err := m.DetectPlatform(ctx, alias)
	if err != nil {
		return false, err
	}
	if platform == PlatformUnix {
		return true, nil
	}

	if snap, ok := m.GetInventorySnapshot(alias); ok && snap.System != nil {
		if strings.Contains(strings.ToLower(snap.System.Shell), "bash") {
			return true, nil
		}
	}

	session, err := m.ensureSession(alias)
	if err != nil {
		return false, err
	}
	ok, evidence, probeErr := session.detectBashAvailability(ctx)
	if probeErr != nil {
		return false, probeErr
	}
	if ok {
		m.mergePlatformToInventory(alias, platform, evidence)
	}
	return ok, nil
}

// mergePlatformToInventory seeds the inventory cache with probed platform/shell data
// so subsequent DetectPlatform calls skip the SSH round-trip.
func (m *Manager) mergePlatformToInventory(alias, platform, evidence string) {
	snap, _ := m.GetInventorySnapshot(alias)
	snap.Alias = alias
	if snap.CollectedAt.IsZero() {
		snap.CollectedAt = time.Now()
	}
	if snap.System == nil {
		snap.System = &inventory.SystemInfo{}
	}
	if strings.TrimSpace(snap.System.OS) == "" && platform != PlatformUnknown {
		snap.System.OS = platform + " (probed)"
	}
	if strings.TrimSpace(evidence) != "" && strings.TrimSpace(snap.System.Shell) == "" {
		snap.System.Shell = evidence
	}
	m.SetInventorySnapshot(alias, snap)
}

func classifyPlatformText(text string) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return PlatformUnknown
	}
	if strings.Contains(lower, "windows") ||
		strings.Contains(lower, "microsoft") ||
		strings.Contains(lower, "win32") {
		return PlatformWindows
	}
	unixHints := []string{
		"linux", "darwin", "macos", "unix", "freebsd", "openbsd", "netbsd", "sunos", "solaris", "aix",
		"/bin/bash", "/bin/sh", "/usr/bin",
	}
	for _, hint := range unixHints {
		if strings.Contains(lower, hint) {
			return PlatformUnix
		}
	}
	return PlatformUnknown
}
