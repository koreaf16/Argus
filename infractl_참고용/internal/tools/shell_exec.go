// Package tools
// File: shell_exec.go
// Description: shell_exec LLM 도구 정의 및 실행 오케스트레이션
// Responsibility: 파라미터 파싱 → 권한 계획 수립 → 실행 경로 분기 → 결과 렌더링

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/privilege"
)

// ShellExecTool executes shell commands on local/remote targets with optional
// privilege escalation and reuse of previously elevated sessions.
type ShellExecTool struct {
	PrivilegeCache *privilege.Cache
	PromptHandler  privilege.PromptHandler
	PrivilegeAudit PrivilegeAuditNotifier
	ApprovalCache  *ApprovalCache
	// IsYoloMode, when non-nil, reports whether YOLO mode is currently active.
	IsYoloMode func() bool
}

type privilegeExecutionPlan struct {
	normalized        NormalizedPrivilegeCommand
	rawInlineApproved bool
	inline            *embeddedPrivilegeSpec
}

func (t *ShellExecTool) Name() string { return "shell_exec" }

func (t *ShellExecTool) Description() string {
	return "Execute shell commands in the current local workspace or a registered SSH workspace. " +
		"`localhost` is the controller machine running infractl and may be Windows, Linux, or macOS. " +
		"Use command syntax that matches the selected target platform. " +
		"For PowerShell directory inventory, use `Get-ChildItem -LiteralPath <dir> -Name`; do not use CMD-only switches such as `dir ... /b`. " +
		"When filenames are unknown, list the directory unfiltered before applying globs or filters. " +
		"Prefer non-interactive flows first (e.g. `sudo -n`, `runuser -l`, `bash -lc`) and only prompt for passwords when required. " +
		"Avoid long-lived interactive shells unless the user explicitly asks to keep the session open. " +
		"Use this when no dedicated tool exists for the task."
}

func (t *ShellExecTool) ModelPrompt() string {
	return "Execute shell commands. On PowerShell, list directories with `Get-ChildItem -LiteralPath <dir> -Name`, not `dir ... /b`; list unknown filenames unfiltered first."
}

func (t *ShellExecTool) IsReadOnly() bool { return false }
func (t *ShellExecTool) IsEnabled() bool  { return true }

func (t *ShellExecTool) IsConcurrencySafe(args map[string]interface{}) bool {
	cmd, _ := argString(args, "command", false)
	if cmd == "" {
		return false
	}
	if isBackground, _ := args["is_background"].(bool); isBackground {
		return true
	}
	return isReadOnlyShellCommand(cmd)
}

func (t *ShellExecTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Command to execute",
			},
			"target": map[string]interface{}{
				"type":        "string",
				"description": "Workspace alias. Omit to use the active workspace. Use 'localhost' ONLY for the local controller machine.",
			},
			"become_method": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"sudo", "su"},
				"description": "Optional privilege method",
			},
			"become_user": map[string]interface{}{
				"type":        "string",
				"description": "Optional privilege target user (default root)",
			},
			"is_background": map[string]interface{}{
				"type":        "boolean",
				"description": "Set to true to run the command in the background.",
			},
		},
		"required": []string{"command"},
	}
}

func (t *ShellExecTool) Execute(ctx context.Context, args map[string]interface{}, exec executor.Executor) (ToolOutcome, error) {
	return t.ExecuteDetailed(ctx, args, exec)
}

func (t *ShellExecTool) ExecuteDetailed(ctx context.Context, args map[string]interface{}, exec executor.Executor) (ToolOutcome, error) {
	cmd, err := argString(args, "command", true)
	if err != nil {
		return ToolOutcome{}, err
	}

	target := strings.TrimSpace(exec.Target())
	if isLocalSCPCommand(cmd) && (target == "" || target == "localhost") {
		return ToolOutcome{
			Content: "Error: scp via shell_exec is not supported; use file_transfer tool instead",
			Success: false, ExitCode: 1, ErrorMessage: "scp via shell_exec is not supported",
		}, nil
	}

	isBackground, _ := args["is_background"].(bool)
	becomeMethodArg, _ := argString(args, "become_method", false)
	becomeUserArg, _ := argString(args, "become_user", false)

	// --- Defensive Tactic Layer ---
	tactic := ApplyDefensiveTactics(cmd, isBackground)
	cmd = tactic.ModifiedCommand

	// --- Pre-flight Check Layer ---
	if pfErr := PerformPreflightChecks(ctx, exec, cmd, becomeMethodArg, becomeUserArg); pfErr != nil {
		return ToolOutcome{
			Content:      fmt.Sprintf("[PRE-CHECK FAILED] %s", pfErr),
			Success:      false,
			ExitCode:     1,
			ErrorMessage: pfErr.Error(),
		}, nil
	}

	privPlan, shortCircuit, err := t.preparePrivilegeExecution(ctx, cmd, becomeMethodArg, becomeUserArg)
	if err != nil {
		return ToolOutcome{}, err
	}
	if shortCircuit != nil {
		return *shortCircuit, nil
	}

	normalized := privPlan.normalized
	cmd = normalized.Command

	// 시스템 파괴 명령 검사 (승인 하네스)
	risk := ScanSystemRisk(cmd)
	if risk.IsSystemDisruptive {
		confirmed, _ := t.confirmRisk(ctx, "System Disruptive Command", risk.Description, cmd)
		if !confirmed {
			return ToolOutcome{
				Content: fmt.Sprintf("[CANCELED] 파괴적인 시스템 명령 실행이 취소되었습니다: %s", risk.Description),
				Success: false, ExitCode: 1, ErrorMessage: risk.Description,
			}, nil
		}
	}

	var auditEventID string
	if t.PrivilegeAudit != nil && (normalized.BecomeMethod != "" || privPlan.rawInlineApproved) {
		auditEventID = t.PrivilegeAudit.NotifyPrivilegeStart(ctx, PrivilegeAuditRequest{
			Command: cmd, NormalizedCmd: normalized.Command, Method: normalized.BecomeMethod, ApprovedBy: "user",
		})
	}

	result, execErr := t.runCommand(ctx, exec, cmd, normalized.BecomeMethod, normalized.BecomeUser, privPlan.inline)
	if auditEventID != "" {
		t.PrivilegeAudit.NotifyPrivilegeEnd(ctx, auditEventID, result.ExitCode, nil, nil)
	}
	if execErr != nil {
		msg := fmt.Sprintf("execution error: %s", execErr)
		return ToolOutcome{Content: msg, Success: false, ExitCode: 1, ErrorMessage: msg}, nil
	}

	out := renderExecResult(exec, result)
	if privPlan.rawInlineApproved {
		out = "[Security Override]\n" + out
	}

	// 방어 로직 경고 추가
	if len(tactic.Warnings) > 0 {
		out = fmt.Sprintf("[Defensive Layer] %s\n%s", strings.Join(tactic.Warnings, "; "), out)
	}

	success, successNote := shellExecSucceeded(cmd, result)
	if successNote != "" {
		out += "\n\n" + successNote
	}

	return ToolOutcome{
		Content:      out,
		Success:      success,
		ExitCode:     result.ExitCode,
		ErrorMessage: strings.TrimSpace(result.Stderr),
		MetadataJSON: buildShellTaskProgressMetadataWithSuccess(args, cmd, result, success).JSON(),
	}, nil
}

func (t *ShellExecTool) confirmRisk(ctx context.Context, title, warning, cmd string) (bool, error) {
	if t.IsYoloMode != nil && t.IsYoloMode() {
		return true, nil
	}
	if t.PromptHandler == nil {
		return false, nil
	}
	req := QuestionRequest{
		Header:   "Security Confirmation",
		Question: fmt.Sprintf("⚠️ %s\n\n위험이 감지되었습니다:\n%s\n\n실행 명령어:\n%s\n\n정말로 이 명령을 실행하시겠습니까?", title, warning, cmd),
		Options: []QuestionOption{
			{Label: "Run Anyway", Description: "위험을 인지했으며 실행을 승인합니다."},
			{Label: "Cancel", Description: "안전을 위해 실행을 취소합니다."},
		},
	}
	resp, err := RequestQuestion(ctx, req)
	if err != nil {
		return false, err
	}
	return resp.SelectedIndex == 0, nil
}

func isReadOnlyShellCommand(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" || strings.ContainsAny(cmd, ">|;&") {
		return false
	}
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return false
	}
	baseCmd := strings.ToLower(fields[0])
	readOnlyCmds := map[string]bool{
		"ls": true, "cat": true, "grep": true, "df": true, "du": true, "ps": true, "who": true, "pwd": true, "hostname": true,
	}
	return readOnlyCmds[baseCmd]
}
