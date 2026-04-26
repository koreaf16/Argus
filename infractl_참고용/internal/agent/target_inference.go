package agent

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/store"
)

func inferTargetFromPathSyntax(
	toolName string,
	args map[string]any,
	activeServer *store.Server,
	manager *executor.Manager,
	localPlatform executor.Platform,
) (string, string) {
	if strings.EqualFold(strings.TrimSpace(toolName), "file_transfer") {
		return "", ""
	}

	pathSample, ok := firstWindowsPathInArgs(args)
	if !ok {
		return "", ""
	}

	localSupportsWindowsPath := localPlatform == executor.PlatformWindows
	activePlatform := activeWorkspacePlatform(activeServer, manager)
	activeSupportsWindowsPath := activeServer != nil && activePlatform == executor.PlatformWindows

	if activeSupportsWindowsPath && !localSupportsWindowsPath {
		return "", ""
	}
	if localSupportsWindowsPath && activeServer == nil {
		return "localhost", ""
	}
	if localSupportsWindowsPath && activeServer != nil && activePlatform != executor.PlatformWindows {
		return "localhost", ""
	}
	if localSupportsWindowsPath && activeSupportsWindowsPath {
		return "", fmt.Sprintf(
			"Error: Windows-style path %q is ambiguous because both localhost and active workspace %q are Windows targets. Specify target:\"localhost\" or target:%q explicitly.",
			pathSample,
			activeServer.Name,
			activeServer.Name,
		)
	}
	if activeServer != nil && activePlatform == executor.PlatformUnknown {
		return "", fmt.Sprintf(
			"Error: Windows-style path %q was provided, but localhost is %s and active workspace %q has unknown platform. Specify an explicit Windows target or move the file to a path readable by this controller.",
			pathSample,
			platformLabel(localPlatform),
			activeServer.Name,
		)
	}

	activeLabel := "(none)"
	if activeServer != nil {
		activeLabel = fmt.Sprintf("%s (%s)", activeServer.Name, platformLabel(activePlatform))
	}
	return "", fmt.Sprintf(
		"Error: Windows-style path %q cannot be routed safely. localhost is %s and active workspace is %s. Use a Windows target, run infractl on Windows, or transfer the file to a path readable by this controller.",
		pathSample,
		platformLabel(localPlatform),
		activeLabel,
	)
}

func inferTargetFromPathSyntaxForRuntime(toolName string, args map[string]any, activeServer *store.Server, manager *executor.Manager) (string, string) {
	return inferTargetFromPathSyntax(toolName, args, activeServer, manager, executor.NormalizePlatform(runtime.GOOS))
}

func firstWindowsPathInArgs(args map[string]any) (string, bool) {
	for _, key := range []string{"command", "path", "source", "remote_path"} {
		if path, ok := firstWindowsPathInValue(args[key]); ok {
			return path, true
		}
	}
	for key, value := range args {
		if !shouldScanArgForPathSyntax(key) {
			continue
		}
		if path, ok := firstWindowsPathInValue(value); ok {
			return path, true
		}
	}
	return "", false
}

func shouldScanArgForPathSyntax(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" || key == "description" || key == "reason" || key == "local_path" {
		return false
	}
	return strings.Contains(key, "path") ||
		strings.Contains(key, "file") ||
		strings.Contains(key, "dir") ||
		strings.Contains(key, "command") ||
		strings.Contains(key, "source")
}

func firstWindowsPathInValue(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return executor.FirstWindowsPath(v)
	case []string:
		for _, item := range v {
			if path, ok := executor.FirstWindowsPath(item); ok {
				return path, true
			}
		}
	case []any:
		for _, item := range v {
			if path, ok := firstWindowsPathInValue(item); ok {
				return path, true
			}
		}
	case map[string]any:
		for _, item := range v {
			if path, ok := firstWindowsPathInValue(item); ok {
				return path, true
			}
		}
	}
	return "", false
}

func activeWorkspacePlatform(activeServer *store.Server, manager *executor.Manager) executor.Platform {
	if activeServer == nil {
		return executor.PlatformUnknown
	}
	if platform := executor.NormalizePlatform(activeServer.OS); platform != executor.PlatformUnknown {
		return platform
	}
	if manager == nil || strings.TrimSpace(activeServer.Name) == "" {
		return executor.PlatformUnknown
	}
	exec, err := manager.Get(activeServer.Name)
	if err != nil {
		return executor.PlatformUnknown
	}
	return executor.CommandPlatform(exec)
}

func platformLabel(platform executor.Platform) string {
	if platform == "" || platform == executor.PlatformUnknown {
		return "unknown"
	}
	return string(platform)
}
