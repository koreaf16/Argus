// Package tools
// File: file_read.go
// Description: 파일 읽기 도구 (권한 승격 지원)
// Responsibility: 대상 시스템의 텍스트 파일을 읽으며, 필요시 sudo/su를 통한 권한 승격 수행

package tools

import (
	"context"
	"fmt"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/privilege"
)

const maxFileReadLines = 500

// FileReadTool은 대상 시스템의 텍스트 파일을 읽는다.
type FileReadTool struct {
	PrivilegeCache *privilege.Cache
	PromptHandler  privilege.PromptHandler
}

func (t *FileReadTool) Name() string { return "file_read" }

func (t *FileReadTool) Description() string {
	return "Read a text file or list a directory on the target system.\n" +
		"Use for: reading config files, log files, scripts, or directory inventory such as install media, Downloads, uploads, and staging folders.\n" +
		"When filenames are unknown, list the directory unfiltered first; apply keyword filters only after seeing the full listing.\n" +
		"Supports privilege escalation via become_method."
}

func (t *FileReadTool) IsReadOnly() bool { return true }
func (t *FileReadTool) IsEnabled() bool  { return true }

func (t *FileReadTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The absolute or relative path to the file or directory",
			},
			"lines": map[string]interface{}{
				"type":        "integer",
				"description": fmt.Sprintf("Maximum number of lines to read (default: all, max: %d)", maxFileReadLines),
			},
			"target": map[string]interface{}{
				"type":        "string",
				"description": "Workspace alias. Omit to use the active workspace. Use 'localhost' ONLY for the local controller machine.",
			},
			"become_method": map[string]interface{}{
				"type":        "string",
				"description": "Privilege escalation method: 'sudo' or 'su'.",
				"enum":        []string{"sudo", "su"},
			},
			"become_user": map[string]interface{}{
				"type":        "string",
				"description": "Target user for privilege escalation (default: root).",
			},
		},
		"required": []string{"path"},
	}
}

func (t *FileReadTool) Execute(ctx context.Context, args map[string]interface{}, exec executor.Executor) (ToolOutcome, error) {
	path, err := argString(args, "path", true)
	if err != nil {
		return ToolOutcome{Content: fmt.Sprintf("Error: %s", err), Success: true}, nil
	}

	lines := argInt(args, "lines", 0)
	if lines > maxFileReadLines {
		lines = maxFileReadLines
	}
	becomeMethod, _ := argString(args, "become_method", false)
	becomeUser, _ := argString(args, "become_user", false)

	cmd := buildReadCommand(exec, path, lines)

	// 1. Try with become if requested
	if becomeMethod != "" {
		if result, ok := executeWithAcquiredPrivilege(ctx, exec, cmd, becomeUser); ok {
			return t.processResult(result)
		}
		result, ok, err := executeWithBecome(ctx, exec, cmd, becomeMethod, becomeUser, t.PrivilegeCache, t.PromptHandler)
		if ok || err != nil {
			if err != nil {
				return ToolOutcome{Content: fmt.Sprintf("Privilege escalation failed: %s", err), Success: true}, nil
			}
			return t.processResult(result)
		}
	}

	// 2. Direct execution
	result, err := exec.Execute(ctx, cmd)
	if err != nil {
		return ToolOutcome{Content: fmt.Sprintf("Execution failed: %s", err), Success: true}, nil
	}

	// 3. Auto-retry via acquired root if permission denied
	if result.ExitCode != 0 && isPermissionFailure(result, nil) {
		if retryResult, ok := executePlainViaAcquiredRoot(ctx, exec, cmd); ok && retryResult.ExitCode == 0 {
			return ToolOutcome{Content: stripPrivilegeReuseNote(retryResult.Stdout), Success: true}, nil
		}
	}
	return t.processResult(result)
}

func (t *FileReadTool) processResult(result executor.ExecResult) (ToolOutcome, error) {
	if result.ExitCode != 0 {
		return ToolOutcome{Content: fmt.Sprintf("Error reading file (exit %d):\n%s", result.ExitCode, result.Stderr), Success: true}, nil
	}
	return ToolOutcome{Content: result.Stdout, Success: true}, nil
}

func buildReadCommand(exec executor.Executor, path string, lines int) string {
	platform := executor.CommandPlatform(exec)
	switch platform {
	case executor.PlatformWindows:
		quoted := executor.QuotePowerShell(path)
		var script string
		if lines > 0 {
			script = fmt.Sprintf(
				"if (Test-Path -LiteralPath %s -PathType Container) { Get-ChildItem -LiteralPath %s -Name } else { Get-Content -LiteralPath %s -TotalCount %d }",
				quoted, quoted, quoted, lines,
			)
		} else {
			script = fmt.Sprintf(
				"if (Test-Path -LiteralPath %s -PathType Container) { Get-ChildItem -LiteralPath %s -Name } else { Get-Content -LiteralPath %s }",
				quoted, quoted, quoted,
			)
		}
		return executor.PowerShellCommand(exec, script)
	default:
		quoted := executor.QuotePOSIX(path)
		if lines > 0 {
			return fmt.Sprintf("if [ -d %s ]; then ls -la %s; else head -n %d %s; fi", quoted, quoted, lines, quoted)
		}
		return fmt.Sprintf("if [ -d %s ]; then ls -la %s; else cat %s; fi", quoted, quoted, quoted)
	}
}
