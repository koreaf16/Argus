package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

// VerifyCompleteTool records that a previously executed task has been verified.
// The agent runtime resolves the referenced shell task and updates its state.
type VerifyCompleteTool struct{}

func (t *VerifyCompleteTool) Name() string {
	return "verify_complete"
}

func (t *VerifyCompleteTool) Description() string {
	return "Record that a previously executed shell task has been verified. Prefer `tool_id` for the matching shell_exec call; use `task_key` when you need a stable task identifier."
}

func (t *VerifyCompleteTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"tool_id": map[string]interface{}{
				"type":        "string",
				"description": "Preferred: the shell_exec tool call ID being closed out.",
			},
			"task_key": map[string]interface{}{
				"type":        "string",
				"description": "Fallback stable task identifier when tool_id is unavailable.",
			},
			"summary": map[string]interface{}{
				"type":        "string",
				"description": "Short final summary of what is now confirmed complete.",
			},
			"verification_evidence": map[string]interface{}{
				"type":        "string",
				"description": "Concrete evidence that the task completed successfully.",
			},
			"auto_chained": map[string]interface{}{
				"type":        "boolean",
				"description": "Runtime-internal flag for automatic chained verification.",
			},
		},
		"required": []string{"summary", "verification_evidence"},
	}
}

func (t *VerifyCompleteTool) IsReadOnly() bool { return false }

func (t *VerifyCompleteTool) IsEnabled() bool {
	return true
}

func (t *VerifyCompleteTool) Execute(ctx context.Context, args map[string]interface{}, exec executor.Executor) (ToolOutcome, error) {
	summary, _ := args["summary"].(string)
	evidence, _ := args["verification_evidence"].(string)
	toolID, _ := args["tool_id"].(string)
	taskKey, _ := args["task_key"].(string)

	if strings.TrimSpace(summary) == "" || strings.TrimSpace(evidence) == "" {
		return ToolOutcome{
			Content: "Error: 'summary' and 'verification_evidence' are required for verify_complete.",
			Success: false,
		}, nil
	}

	target := strings.TrimSpace(toolID)
	if target == "" {
		target = strings.TrimSpace(taskKey)
	}
	if target == "" {
		target = "latest pending task"
	}

	prefix := ""
	if autoChained, _ := args["auto_chained"].(bool); autoChained {
		prefix = "[자동 후속 검증]\n"
	}

	return ToolOutcome{
		Content: prefix + fmt.Sprintf("Verification recorded for %s\nSummary: %s\nEvidence: %s", target, summary, evidence),
		Success: true,
	}, nil
}
