// Package tools
// File: submit_job.go
// Description: submit_job LLM 도구 정의 및 비동기 실행 오케스트레이션
// Responsibility: 장시간 실행되는 셸 명령을 백그라운드 작업으로 등록하고 작업 ID를 즉시 반환

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/background"
	"github.com/yourorg/infractl/internal/executor"
)

// SubmitJobTool submits a command to run asynchronously in the background.
type SubmitJobTool struct {
	Manager *background.Manager
}

func (t *SubmitJobTool) Name() string { return "submit_job" }

func (t *SubmitJobTool) Description() string {
	return "Submit a long-running shell command (e.g., installations, large unzips, backups) to run asynchronously in the background. " +
		"Returns a Job ID immediately. The system will notify you when the job completes. " +
		"Do not use this for quick commands; use `shell_exec` instead."
}

func (t *SubmitJobTool) ModelPrompt() string {
	return "Submit a command to the background job manager. " +
		"Use this for ANY command that might take longer than a few minutes. " +
		"You will receive a Job ID immediately and can proceed with other tasks while it runs. " +
		"The system will automatically alert you upon completion."
}

func (t *SubmitJobTool) IsReadOnly() bool { return false }
func (t *SubmitJobTool) IsEnabled() bool {
	return t.Manager != nil
}

func (t *SubmitJobTool) IsConcurrencySafe(args map[string]interface{}) bool {
	return true
}

func (t *SubmitJobTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The long-running shell command to execute in the background",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "A short, clear description of the job (e.g., 'Installing Oracle 19c RU patch')",
			},
			"target": map[string]interface{}{
				"type":        "string",
				"description": "Registered server alias. Omit to use the active server.",
			},
		},
		"required": []string{"command", "description"},
	}
}

func (t *SubmitJobTool) Execute(ctx context.Context, args map[string]interface{}, exec executor.Executor) (ToolOutcome, error) {
	if t.Manager == nil {
		return ToolOutcome{Content: "Error: background manager is not configured", Success: false, ExitCode: 1, ErrorMessage: "no manager"}, nil
	}

	cmd, err := argString(args, "command", true)
	if err != nil {
		return ToolOutcome{}, err
	}

	desc, err := argString(args, "description", true)
	if err != nil {
		return ToolOutcome{}, err
	}

	// Submit the job to the background manager
	jobID := t.Manager.SubmitStreaming(context.Background(), desc, func(jobCtx context.Context, streams *background.JobStreams) error {
		// Use a separate context to ensure it runs independently of the LLM turn
		// We execute the command through the provided executor
		result, execErr := exec.Execute(jobCtx, cmd)

		if streams != nil && streams.Stdout != nil {
			_, _ = fmt.Fprintln(streams.Stdout, result.Stdout)
		}
		if streams != nil && streams.Stderr != nil {
			_, _ = fmt.Fprintln(streams.Stderr, result.Stderr)
		}

		if execErr != nil {
			return execErr
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("command exited with code %d: %s", result.ExitCode, strings.TrimSpace(result.Stderr))
		}
		return nil
	})

	content := fmt.Sprintf("Successfully submitted background job.\nJob ID: %d\nDescription: %s\nCommand: %s\n\nThe system will automatically notify you when this job completes or fails.", jobID, desc, cmd)
	meta := TaskProgressMetadata{
		TaskKey:              fmt.Sprintf("job:%d", jobID),
		JobID:                jobID,
		ExecutionStatus:      "submitted",
		VerificationRequired: true,
		VerificationStatus:   "pending",
	}

	return ToolOutcome{
		Content:      content,
		Success:      true,
		MetadataJSON: meta.JSON(),
	}, nil
}
