// Package executor
// File: detect.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package executor

import (
	"context"
	"strings"

	"github.com/yourorg/infractl/internal/executor/shell/powershell"
)

// DetectOS probes the target and returns a human-readable OS description plus platform family.
func DetectOS(ctx context.Context, exec Executor) (string, Platform) {
	if exec == nil {
		return "", PlatformUnknown
	}

	unixOSRelease := `cat /etc/os-release 2>/dev/null | grep -E '^PRETTY_NAME=' | head -1 | cut -d'=' -f2 | tr -d '"'`
	if res, err := exec.Execute(ctx, unixOSRelease); err == nil && strings.TrimSpace(res.Stdout) != "" {
		out := strings.TrimSpace(res.Stdout)
		return out, NormalizePlatform(out)
	}

	psCmd := `(Get-CimInstance Win32_OperatingSystem).Caption`
	encoded := powershell.EncodeUTF16LE(psCmd)
	windowsCaption := `powershell -NoProfile -NonInteractive -EncodedCommand ` + encoded
	if res, err := exec.Execute(ctx, windowsCaption); err == nil && strings.TrimSpace(res.Stdout) != "" {
		out := strings.TrimSpace(res.Stdout)
		return out, NormalizePlatform(out)
	}

	if res, err := exec.Execute(ctx, "ver"); err == nil && strings.TrimSpace(res.Stdout) != "" {
		out := strings.TrimSpace(res.Stdout)
		if NormalizePlatform(out) == PlatformWindows {
			return out, PlatformWindows
		}
	}

	if res, err := exec.Execute(ctx, "uname -srm"); err == nil && strings.TrimSpace(res.Stdout) != "" {
		out := strings.TrimSpace(res.Stdout)
		return out, NormalizePlatform(out)
	}

	return "", PlatformUnknown
}

