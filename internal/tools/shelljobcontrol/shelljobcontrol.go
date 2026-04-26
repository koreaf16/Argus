package shelljobcontrol

import (
	"encoding/json"
	"fmt"
	"strings"

	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
)

type ShellJobControlTool struct{}

func NewShellJobControlTool() *ShellJobControlTool {
	return &ShellJobControlTool{}
}

func (t *ShellJobControlTool) Name() string {
	return "shell_job_control"
}

func (t *ShellJobControlTool) Description(ctx tool.Context) string {
	return "Control background shell jobs. Actions: stop, send_input."
}

func (t *ShellJobControlTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "One of: stop, send_input.",
			},
			"job_id": map[string]any{
				"type":        "string",
				"description": "Target job id.",
			},
			"input": map[string]any{
				"type":        "string",
				"description": "Input text for send_input action.",
			},
		},
		"required": []string{"action", "job_id"},
	}
}

func (t *ShellJobControlTool) IsReadOnly() bool {
	return false
}

func (t *ShellJobControlTool) MaxResultSizeChars() int {
	return 100000
}

func (t *ShellJobControlTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	out := make(chan tool.ToolEvent, 2)
	if ctx.ShellJobs == nil {
		return nil, fmt.Errorf("shell job manager is unavailable")
	}

	var req struct {
		Action string `json:"action"`
		JobID  string `json:"job_id"`
		Input  string `json:"input"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	go func() {
		defer close(out)
		action := strings.ToLower(strings.TrimSpace(req.Action))
		jobID := strings.TrimSpace(req.JobID)
		if jobID == "" {
			out <- tool.NewErrorEvent(fmt.Errorf("job_id is required"))
			return
		}

		switch action {
		case "stop":
			if err := ctx.ShellJobs.Stop(jobID); err != nil {
				out <- tool.NewErrorEvent(err)
				return
			}
			out <- tool.NewOutputEvent(fmt.Sprintf("stop requested for %s", jobID))
			out <- tool.NewDoneEvent()
			return
		case "send_input":
			if err := ctx.ShellJobs.SendInput(jobID, req.Input); err != nil {
				out <- tool.NewErrorEvent(err)
				return
			}
			out <- tool.NewOutputEvent(fmt.Sprintf("input sent to %s", jobID))
			out <- tool.NewDoneEvent()
			return
		default:
			out <- tool.NewErrorEvent(fmt.Errorf("unknown action: %s", action))
			return
		}
	}()

	return out, nil
}

func (t *ShellJobControlTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	return tool.PermissionResult{
		Behavior: types.BehaviorAsk,
		Message:  "shell_job_control requires confirmation",
	}, nil
}
