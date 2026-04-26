// Package tools
// File: file_write.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yourorg/infractl/internal/diff"
	"github.com/yourorg/infractl/internal/executor"
)

// FileWriteTool writes or appends content to a file on the target system.
type FileWriteTool struct{}

func (t *FileWriteTool) Name() string { return "file_write" }

func (t *FileWriteTool) Description() string {
	return "Write or append content to a file on the target system.\n" +
		"Use for: creating scripts, modifying config files, writing output to disk.\n" +
		"Prefer small targeted writes; avoid overwriting critical system files without confirmation."
}

func (t *FileWriteTool) IsReadOnly() bool { return false }
func (t *FileWriteTool) IsEnabled() bool  { return true }

func (t *FileWriteTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The absolute or relative path to the file",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The content to write to the file",
			},
			"append": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, append to existing file; if false (default), overwrite",
			},
			"target": map[string]interface{}{
				"type":        "string",
				"description": "Workspace alias. Omit or use 'localhost' for the local workspace.",
			},
			"skip_backup": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, skip automatic .bak backup before overwrite. Use only when disk space is insufficient.",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *FileWriteTool) Execute(ctx context.Context, args map[string]interface{}, exec executor.Executor) (ToolOutcome, error) {
	path, err := argString(args, "path", true)
	if err != nil {
		return ToolOutcome{Content: fmt.Sprintf("Error: %s", err), Success: true}, nil
	}

	content, err := argString(args, "content", true)
	if err != nil {
		return ToolOutcome{Content: fmt.Sprintf("Error: %s", err), Success: true}, nil
	}

	appendMode := argBool(args, "append", false)
	skipBackup := argBool(args, "skip_backup", false)

	oldContent := readExistingFile(ctx, exec, path)

	var criticalWarning string
	if warning, _ := CheckCriticalPath(path); warning != "" {
		criticalWarning = warning
	}

	var backupMsg string
	if !appendMode && oldContent != "" && !skipBackup {
		spaceCheckCmd := buildFileSpaceCheckCmd(exec, path)
		spaceRes, spaceErr := exec.Execute(ctx, spaceCheckCmd)
		if spaceErr == nil && strings.HasPrefix(strings.TrimSpace(spaceRes.Stdout), "INSUFFICIENT:") {
			parts := strings.SplitN(strings.TrimSpace(spaceRes.Stdout), ":", 3)
			if len(parts) == 3 {
				return ToolOutcome{Content: fmt.Sprintf(
					"Backup requires more space than is available (needed: %sKB, available: %sKB).\nSet skip_backup=true only when you intentionally accept the rollback risk.",
					parts[1], parts[2],
				), Success: true}, nil
			}
		}

		bakCmd := buildBackupCmd(exec, path)
		bakRes, bakErr := exec.Execute(ctx, bakCmd)
		if bakErr != nil || bakRes.ExitCode != 0 {
			backupMsg = fmt.Sprintf("[Warning] backup %s.bak failed", path)
		} else {
			backupMsg = fmt.Sprintf("[Backup] %s.bak", path)
			args["rollback_command"] = buildRestoreBackupCmd(exec, path)
		}
	}

	cmd := buildWriteCommand(exec, path, content, appendMode)
	result, err := exec.Execute(ctx, cmd)
	if err != nil {
		return ToolOutcome{Content: fmt.Sprintf("Execution failed: %s", err), Success: true}, nil
	}

	if result.ExitCode != 0 {
		if isPermissionFailure(result, nil) {
			if retryResult, ok := executePlainViaAcquiredRoot(ctx, exec, cmd); ok && retryResult.ExitCode == 0 {
				action := "written"
				if appendMode {
					action = "appended"
				}
				msg := fmt.Sprintf("File %s %s successfully via acquired root session", path, action)
				if criticalWarning != "" {
					msg = criticalWarning + "\n" + msg
				}
				if backupMsg != "" {
					msg = backupMsg + "\n" + msg
				}
				return ToolOutcome{Content: msg, Success: true}, nil
			}
		}
		return ToolOutcome{Content: fmt.Sprintf("Error writing file (exit %d):\n%s", result.ExitCode, result.Stderr), Success: true}, nil
	}

	action := "written"
	newContent := content
	if appendMode {
		action = "appended"
		newContent = oldContent + content
	}

	msg := fmt.Sprintf("File %s %s successfully", path, action)
	if criticalWarning != "" {
		msg = criticalWarning + "\n" + msg
	}
	if backupMsg != "" {
		msg = backupMsg + "\n" + msg
	}

	filename := filepath.Base(path)
	unifiedDiff := diff.GenerateUnifiedDiff(oldContent, newContent, filename)
	if unifiedDiff != "" {
		return ToolOutcome{Content: msg + "\n\n" + unifiedDiff, Success: true}, nil
	}
	return ToolOutcome{Content: msg, Success: true}, nil
}

func readExistingFile(ctx context.Context, exec executor.Executor, path string) string {
	cmd := buildReadCommand(exec, path, 0)
	result, err := exec.Execute(ctx, cmd)
	if err != nil || result.ExitCode != 0 {
		return ""
	}
	return result.Stdout
}

func buildWriteCommand(exec executor.Executor, path, content string, appendMode bool) string {
	switch executor.CommandPlatform(exec) {
	case executor.PlatformWindows:
		return buildWindowsWriteCommand(exec, path, content, appendMode)
	default:
		return buildUnixWriteCommand(path, content, appendMode)
	}
}

func buildUnixWriteCommand(path, content string, appendMode bool) string {
	redirect := ">"
	if appendMode {
		redirect = ">>"
	}

	delimiter := "INFRACTL_EOF"
	for strings.Contains(content, delimiter) {
		delimiter += "_X"
	}

	return fmt.Sprintf(
		"cat <<'%s' %s %s\n%s\n%s",
		delimiter,
		redirect,
		executor.QuotePOSIX(path),
		content,
		delimiter,
	)
}

func buildWindowsWriteCommand(exec executor.Executor, path, content string, appendMode bool) string {
	pathLit := executor.QuotePowerShell(path)
	script := ""
	if appendMode {
		script = fmt.Sprintf(
			"Add-Content -LiteralPath %s -Value @'\n%s\n'@",
			pathLit,
			content,
		)
	} else {
		script = fmt.Sprintf(
			"Set-Content -LiteralPath %s -Value @'\n%s\n'@",
			pathLit,
			content,
		)
	}
	return executor.PowerShellCommand(exec, script)
}

func buildFileSpaceCheckCmd(exec executor.Executor, path string) string {
	platform := executor.CommandPlatform(exec)
	switch platform {
	case executor.PlatformWindows:
		script := fmt.Sprintf(
			"$target=%s; $item=Get-Item -LiteralPath $target -ErrorAction SilentlyContinue; $needed=0; if ($item -and -not $item.PSIsContainer) { $needed=[math]::Ceiling($item.Length / 1KB) }; $dir=''; if ($item) { if ($item.PSIsContainer) { $dir=$item.FullName } else { $dir=$item.Directory.FullName } } else { $dir=Split-Path -LiteralPath $target -Parent }; if ([string]::IsNullOrWhiteSpace($dir)) { $dir=(Get-Location).Path }; $probe=Get-Item -LiteralPath $dir -ErrorAction SilentlyContinue; if (-not $probe) { 'OK:0'; exit 0 }; $available=[math]::Floor($probe.PSDrive.Free / 1KB); if ($needed -gt $available) { 'INSUFFICIENT:' + $needed + ':' + $available } else { 'OK:' + $available }",
			executor.QuotePowerShell(path),
		)
		return executor.PowerShellCommand(exec, script)
	default:
		return fmt.Sprintf(
			`available=$(df -k "$(dirname %s)" 2>/dev/null | awk 'NR==2{print $4}'); needed=$(du -k %s 2>/dev/null | cut -f1 || echo 0); [ -z "$available" ] && echo "OK:0" || { [ "$needed" -gt "$available" ] && echo "INSUFFICIENT:${needed}:${available}" || echo "OK:${available}"; }`,
			executor.QuotePOSIX(path),
			executor.QuotePOSIX(path),
		)
	}
}

func buildBackupCmd(exec executor.Executor, path string) string {
	switch executor.CommandPlatform(exec) {
	case executor.PlatformWindows:
		script := fmt.Sprintf(
			"Copy-Item -LiteralPath %s -Destination %s -Force",
			executor.QuotePowerShell(path),
			executor.QuotePowerShell(path+".bak"),
		)
		return executor.PowerShellCommand(exec, script)
	default:
		return fmt.Sprintf("cp -p %s %s", executor.QuotePOSIX(path), executor.QuotePOSIX(path+".bak"))
	}
}

func buildRestoreBackupCmd(exec executor.Executor, path string) string {
	switch executor.CommandPlatform(exec) {
	case executor.PlatformWindows:
		script := fmt.Sprintf(
			"Copy-Item -LiteralPath %s -Destination %s -Force",
			executor.QuotePowerShell(path+".bak"),
			executor.QuotePowerShell(path),
		)
		return executor.PowerShellCommand(exec, script)
	default:
		return fmt.Sprintf("cp -p %s %s", executor.QuotePOSIX(path+".bak"), executor.QuotePOSIX(path))
	}
}
