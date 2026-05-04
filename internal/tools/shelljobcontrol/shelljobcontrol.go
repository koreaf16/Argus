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
	return "백그라운드 쉘 작업을 제어합니다. 액션: stop, send_input."
}

func (t *ShellJobControlTool) IsVisible(ctx tool.Context) bool {
	return ctx.ShellJobs != nil && len(ctx.ShellJobs.List()) > 0
}

func (t *ShellJobControlTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "다음 중 하나: stop, send_input.",
			},
			"job_id": map[string]any{
				"type":        "string",
				"description": "대상 작업 ID.",
			},
			"input": map[string]any{
				"type":        "string",
				"description": "send_input 액션을 위한 입력 텍스트.",
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
		return nil, fmt.Errorf("쉘 작업 관리자를 사용할 수 없습니다")
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
			out <- tool.NewErrorEvent(fmt.Errorf("job_id가 필요합니다"))
			return
		}

		switch action {
		case "stop":
			if err := ctx.ShellJobs.Stop(jobID); err != nil {
				out <- tool.NewErrorEvent(err)
				return
			}
			out <- tool.NewOutputEvent(fmt.Sprintf("%s에 대해 중지가 요청되었습니다", jobID))
			out <- tool.NewDoneEvent()
			return
		case "send_input":
			if err := ctx.ShellJobs.SendInput(jobID, req.Input); err != nil {
				out <- tool.NewErrorEvent(err)
				return
			}
			out <- tool.NewOutputEvent(fmt.Sprintf("%s에 입력이 전송되었습니다", jobID))
			out <- tool.NewDoneEvent()
			return
		default:
			out <- tool.NewErrorEvent(fmt.Errorf("알 수 없는 액션: %s", action))
			return
		}
	}()

	return out, nil
}

func (t *ShellJobControlTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	return tool.PermissionResult{
		Behavior: types.BehaviorAsk,
		Message:  "shell_job_control은 확인이 필요합니다",
	}, nil
}
