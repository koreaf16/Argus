// Package tools
// File: sysctl_manage.go
// Description: 커널 파라미터(sysctl) 관리 전용 도구
// Responsibility: sysctl -w를 통한 즉시 반영 및 /etc/sysctl.conf 영구 저장을 수행 (체크포인트 지원)

package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/privilege"
)

// SysctlManageTool은 커널 파라미터를 관리한다.
type SysctlManageTool struct {
	PrivilegeCache    *privilege.Cache
	PromptHandler     privilege.PromptHandler
	ApprovalCache     *ApprovalCache
	CheckpointManager CheckpointCreator
}

func (t *SysctlManageTool) Name() string { return "sysctl_manage" }

func (t *SysctlManageTool) Description() string {
	return "Manage Linux kernel parameters (sysctl).\n" +
		"Applies changes immediately with 'sysctl -w' and persists them in '/etc/sysctl.conf'.\n" +
		"Supports privilege escalation via become_method."
}

func (t *SysctlManageTool) IsReadOnly() bool { return false }
func (t *SysctlManageTool) IsEnabled() bool  { return true }

func (t *SysctlManageTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"key": map[string]interface{}{
				"type":        "string",
				"description": "Kernel parameter key (e.g., fs.file-max)",
			},
			"value": map[string]interface{}{
				"type":        "string",
				"description": "New value for the parameter",
			},
			"persist": map[string]interface{}{
				"type":        "boolean",
				"default":     true,
				"description": "Whether to persist the change in /etc/sysctl.conf",
			},
			"become_method": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"sudo", "su"},
				"default":     "sudo",
				"description": "Privilege escalation method",
			},
			"become_user": map[string]interface{}{
				"type":        "string",
				"description": "Target user (default: root)",
			},
			"target": map[string]interface{}{
				"type":        "string",
				"description": "Target workspace alias. Omit to use the active workspace. Use 'localhost' ONLY for the local controller machine.",
			},
		},
		"required": []string{"key", "value"},
	}
}

func (t *SysctlManageTool) Execute(ctx context.Context, args map[string]interface{}, exec executor.Executor) (ToolOutcome, error) {
	key, _ := argString(args, "key", true)
	value, _ := argString(args, "value", true)
	persist := argBool(args, "persist", true)
	becomeMethod, _ := argString(args, "become_method", false)
	becomeUser, _ := argString(args, "become_user", false)

	// Create checkpoint for rollback
	if t.CheckpointManager != nil {
		t.CheckpointManager.CreateMandatory(ctx, exec.Target(), t.Name(), args)
	}

	// Safety check: confirm risk
	confirmed, _ := t.confirmRisk(ctx, exec, "Kernel Parameter Modification", fmt.Sprintf("Modify %s = %s", key, value), key, value)
	if !confirmed {
		return ToolOutcome{Content: "[CANCELED] Operation aborted by user.", Success: false}, nil
	}

	// 1. Immediate apply
	applyCmd := fmt.Sprintf("sysctl -w %s=%s", key, value)
	
	// 2. Persistence
	var finalCmd string
	if persist {
		persistCmd := fmt.Sprintf(`grep -qE '^%s\s*=' /etc/sysctl.conf && sed -i 's/^%s\s*=.*$/%s = %s/' /etc/sysctl.conf || echo '%s = %s' >> /etc/sysctl.conf`, 
			strings.ReplaceAll(key, ".", "\\."), strings.ReplaceAll(key, ".", "\\."), key, value, key, value)
		finalCmd = fmt.Sprintf("%s && %s", applyCmd, persistCmd)
	} else {
		finalCmd = applyCmd
	}

	var result executor.ExecResult
	var err error
	if becomeMethod != "" {
		if res, ok := executeWithAcquiredPrivilege(ctx, exec, finalCmd, becomeUser); ok {
			result = res
		} else {
			res, ok, bErr := executeWithBecome(ctx, exec, finalCmd, becomeMethod, becomeUser, t.PrivilegeCache, t.PromptHandler)
			if ok || bErr != nil {
				result = res
				err = bErr
			} else {
				result, err = exec.Execute(ctx, finalCmd)
			}
		}
	} else {
		result, err = exec.Execute(ctx, finalCmd)
	}

	if err != nil {
		return ToolOutcome{Content: fmt.Sprintf("Execution failed: %s", err), Success: false}, nil
	}
	if result.ExitCode != 0 {
		return ToolOutcome{Content: fmt.Sprintf("Error managing sysctl (exit %d):\n%s", result.ExitCode, result.Stderr), Success: false}, nil
	}

	return ToolOutcome{Content: fmt.Sprintf("✓ Successfully set %s = %s (applied and persisted)", key, value), Success: true}, nil
}

func (t *SysctlManageTool) confirmRisk(ctx context.Context, exec executor.Executor, title, warning, key, value string) (bool, error) {
	target := exec.Target()
	if t.ApprovalCache.IsApproved(target, "global_risk") {
		return true, nil
	}
	if t.PromptHandler == nil {
		return false, nil
	}
	req := QuestionRequest{
		Header:   "Security Confirmation",
		Question: fmt.Sprintf("⚠️ %s\n\n위험이 감지되었습니다:\n%s\n\n설정 파라미터:\n%s = %s\n\n정말로 이 커널 설정을 변경하시겠습니까?", title, warning, key, value),
		Options: []QuestionOption{
			{Label: "Run Anyway", Description: "실행합니다."},
			{Label: "Approve for 5m", Description: "향후 5분간 이 타겟의 모든 위험 작업을 승인합니다."},
			{Label: "Cancel", Description: "중단합니다."},
		},
	}
	resp, err := RequestQuestion(ctx, req)
	if err != nil { return false, err }
	if resp.SelectedIndex == 0 { return true, nil }
	if resp.SelectedIndex == 1 {
		t.ApprovalCache.Grant(target, "global_risk", 5*time.Minute)
		return true, nil
	}
	return false, nil
}
