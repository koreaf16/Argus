// Package tools
// File: file_patch.go
// Description: 파일 내 특정 문자열을 찾아 다른 문자열로 교체하는 정교한 편집 도구
// Responsibility: old_string을 new_string으로 교체하고, 수정 전후의 Unified Diff를 생성하여 반환

package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yourorg/infractl/internal/diff"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/privilege"
)

// FilePatchTool은 파일 내의 특정 문자열 블록을 찾아 교체한다.
type FilePatchTool struct {
	PrivilegeCache    *privilege.Cache
	PromptHandler     privilege.PromptHandler
	ApprovalCache     *ApprovalCache
	CheckpointManager CheckpointCreator
}

func (t *FilePatchTool) Name() string { return "file_patch" }

func (t *FilePatchTool) Description() string {
	return "Replace a specific block of text in a file with new content.\n" +
		"Use this for precise configuration edits. It fails if the 'old_string' is not found or is ambiguous.\n" +
		"Always provides a visual +/- diff of the changes."
}

func (t *FilePatchTool) IsReadOnly() bool { return false }
func (t *FilePatchTool) IsEnabled() bool  { return true }

func (t *FilePatchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Absolute path to the file to edit.",
			},
			"old_string": map[string]interface{}{
				"type":        "string",
				"description": "The exact literal text to be replaced.",
			},
			"new_string": map[string]interface{}{
				"type":        "string",
				"description": "The replacement text.",
			},
			"allow_multiple": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, replace all occurrences of old_string. If false (default), fail if multiple occurrences are found.",
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
			"target": map[string]interface{}{
				"type":        "string",
				"description": "Target server alias. Omit to use the active server.",
			},
		},
		"required": []string{"path", "old_string", "new_string"},
	}
}

func (t *FilePatchTool) Execute(ctx context.Context, args map[string]interface{}, exec executor.Executor) (ToolOutcome, error) {
	path, _ := argString(args, "path", true)
	oldStr, _ := argString(args, "old_string", true)
	newStr, _ := argString(args, "new_string", true)
	allowMultiple := argBool(args, "allow_multiple", false)
	becomeMethod, _ := argString(args, "become_method", false)
	becomeUser, _ := argString(args, "become_user", false)

	// Read existing content
	oldFullContent := readExistingFile(ctx, exec, path)
	if oldFullContent == "" {
		return ToolOutcome{Content: fmt.Sprintf("Error: File %s not found or empty.", path), Success: false}, nil
	}

	count := strings.Count(oldFullContent, oldStr)
	if count == 0 {
		return ToolOutcome{Content: fmt.Sprintf("Error: 'old_string' not found in %s.", path), Success: false}, nil
	}
	if count > 1 && !allowMultiple {
		return ToolOutcome{Content: fmt.Sprintf("Error: Multiple occurrences of 'old_string' found (%d). Set 'allow_multiple: true' to replace all.", count), Success: false}, nil
	}

	// Perform replacement in memory
	newFullContent := ""
	if allowMultiple {
		newFullContent = strings.ReplaceAll(oldFullContent, oldStr, newStr)
	} else {
		newFullContent = strings.Replace(oldFullContent, oldStr, newStr, 1)
	}

	if oldFullContent == newFullContent {
		return ToolOutcome{Content: "No changes detected (content already matches).", Success: true}, nil
	}

	// Create checkpoint for rollback
	if t.CheckpointManager != nil {
		t.CheckpointManager.CreateMandatory(ctx, exec.Target(), t.Name(), args)
	}

	// Write the new content back
	cmd := buildWriteCommand(exec, path, newFullContent, false) // Overwrite with patched content

	var result executor.ExecResult
	var err error

	if becomeMethod != "" {
		if res, ok := executeWithAcquiredPrivilege(ctx, exec, cmd, becomeUser); ok {
			result = res
		} else {
			result, _, err = executeWithBecome(ctx, exec, cmd, becomeMethod, becomeUser, t.PrivilegeCache, t.PromptHandler)
		}
	} else {
		result, err = exec.Execute(ctx, cmd)
	}

	if err != nil || result.ExitCode != 0 {
		return ToolOutcome{Content: fmt.Sprintf("Error applying patch: %v\n%s", err, result.Stderr), Success: false}, nil
	}

	// Generate and return Diff
	unifiedDiff := diff.GenerateUnifiedDiff(oldFullContent, newFullContent, filepath.Base(path))
	msg := fmt.Sprintf("Successfully patched %s", path)
	return ToolOutcome{Content: msg + "\n\n" + unifiedDiff, Success: true}, nil
}
