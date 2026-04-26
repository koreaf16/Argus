package tools

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/koreaf16/argus/internal/services/workspace"
	"github.com/koreaf16/argus/internal/utils"
)

type ShellTargetInfo struct {
	Alias         string
	Platform      string
	BashAvailable bool
}

func ResolveWorkspaceAlias(ctx Context, requested string) string {
	if ctx.Workspace == nil {
		if strings.TrimSpace(requested) != "" {
			return strings.ToLower(strings.TrimSpace(requested))
		}
		return workspace.LocalAlias
	}
	if strings.TrimSpace(requested) == "" {
		return ctx.Workspace.ActiveAlias()
	}
	return ctx.Workspace.ResolveAlias(requested)
}

func ResolveValidatedWorkspaceAlias(ctx Context, requested string) (string, error) {
	alias := ResolveWorkspaceAlias(ctx, requested)
	if strings.TrimSpace(alias) == "" {
		alias = workspace.LocalAlias
	}
	if strings.TrimSpace(requested) == "" {
		return alias, nil
	}
	if ctx.Workspace == nil {
		return "", fmt.Errorf("workspace manager is unavailable")
	}
	if _, ok := ctx.Workspace.Registry().Get(alias); !ok {
		return "", fmt.Errorf("unknown server alias: %s", alias)
	}
	return alias, nil
}

func ResolveShellTargetInfo(ctx Context, requested string, probe bool) (ShellTargetInfo, error) {
	alias, err := ResolveValidatedWorkspaceAlias(ctx, requested)
	if err != nil {
		return ShellTargetInfo{}, err
	}
	info := ShellTargetInfo{
		Alias:    alias,
		Platform: workspace.PlatformUnknown,
	}

	if alias == workspace.LocalAlias || ctx.Workspace == nil {
		if runtime.GOOS == "windows" {
			info.Platform = workspace.PlatformWindows
			info.BashAvailable = localWindowsHasBash()
		} else {
			info.Platform = workspace.PlatformUnix
			info.BashAvailable = true
		}
		return info, nil
	}

	if snap, ok := ctx.Workspace.GetInspectSnapshot(alias); ok {
		info.Platform = classifyPlatformFromSnapshot(snap)
		info.BashAvailable = snapshotHasBash(snap)
		if info.Platform == workspace.PlatformUnix {
			info.BashAvailable = true
		}
	}

	if info.Platform == workspace.PlatformUnknown && probe {
		platform, detectErr := ctx.Workspace.DetectPlatform(ctx.Context, alias)
		if detectErr != nil {
			return ShellTargetInfo{}, detectErr
		}
		info.Platform = platform
	}

	if probe {
		if info.Platform == workspace.PlatformUnix {
			info.BashAvailable = true
		} else if !info.BashAvailable && info.Platform == workspace.PlatformWindows {
			hasBash, bashErr := ctx.Workspace.DetectBashAvailability(ctx.Context, alias)
			if bashErr != nil {
				return ShellTargetInfo{}, bashErr
			}
			info.BashAvailable = hasBash
		}
	}

	return info, nil
}

func ValidateShellCompatibility(shellKind string, info ShellTargetInfo) error {
	kind := strings.ToLower(strings.TrimSpace(shellKind))
	switch kind {
	case "bash":
		if info.Platform == workspace.PlatformWindows && !info.BashAvailable {
			return fmt.Errorf("bash is not available on target %s (windows). Use powershell for this target", info.Alias)
		}
	case "powershell":
		if info.Platform == workspace.PlatformUnix {
			return fmt.Errorf("powershell is not supported on target %s (unix-like). Use bash for this target", info.Alias)
		}
	}

	if info.Platform == workspace.PlatformUnknown {
		return fmt.Errorf("could not determine target OS for %s. Run server_inspect and retry", info.Alias)
	}
	return nil
}

func IsRemoteWorkspace(ctx Context, alias string) bool {
	if ctx.Workspace == nil {
		return false
	}
	return ctx.Workspace.IsRemoteAlias(alias)
}

func POSIXShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func PromptPasswordThroughEvent(ctx context.Context, events chan<- ToolEvent, alias, kind, prompt string) (string, error) {
	if events == nil {
		return "", fmt.Errorf("password prompt channel is unavailable")
	}
	label := strings.TrimSpace(prompt)
	if label == "" {
		label = fmt.Sprintf("%s password:", kind)
	}
	pwCh := make(chan string, 1)
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case events <- ToolEvent{
		Kind:             ToolEventPasswordPrompt,
		Prompt:           fmt.Sprintf("[%s] %s", alias, label),
		PasswordResponse: pwCh,
	}:
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case pw := <-pwCh:
		return pw, nil
	}
}

func localWindowsHasBash() bool {
	sh, err := utils.FindSuitableShell()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(sh), "bash")
}

func classifyPlatformFromSnapshot(snap workspace.InspectSnapshot) string {
	return classifyPlatformText(snap.OS + "\n" + snap.Shell)
}

func snapshotHasBash(snap workspace.InspectSnapshot) bool {
	text := strings.ToLower(snap.Shell + "\n" + snap.OS)
	return strings.Contains(text, "bash")
}

func classifyPlatformText(text string) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return workspace.PlatformUnknown
	}
	if strings.Contains(lower, "windows") ||
		strings.Contains(lower, "microsoft") ||
		strings.Contains(lower, "win32") {
		return workspace.PlatformWindows
	}
	unixHints := []string{
		"linux", "darwin", "macos", "unix", "freebsd", "openbsd", "netbsd", "sunos", "solaris", "aix",
		"/bin/bash", "/bin/sh", "/usr/bin",
	}
	for _, hint := range unixHints {
		if strings.Contains(lower, hint) {
			return workspace.PlatformUnix
		}
	}
	return workspace.PlatformUnknown
}
