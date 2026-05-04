package shelljob

import (
	"encoding/json"
	"fmt"
	"strings"

	tool "github.com/koreaf16/argus/internal/tools"
)

type ShellJobTool struct{}

func NewShellJobTool() *ShellJobTool {
	return &ShellJobTool{}
}

func (t *ShellJobTool) Name() string {
	return "shell_job"
}

func (t *ShellJobTool) Description(ctx tool.Context) string {
	return "백그라운드 쉘 작업을 검사합니다. 액션: list, status, tail."
}

func (t *ShellJobTool) IsVisible(ctx tool.Context) bool {
	return ctx.ShellJobs != nil && len(ctx.ShellJobs.List()) > 0
}

func (t *ShellJobTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "다음 중 하나: list, status, tail.",
			},
			"job_id": map[string]any{
				"type":        "string",
				"description": "status와 tail 액션에 필수입니다.",
			},
			"tail_chars": map[string]any{
				"type":        "integer",
				"description": "tail 액션을 위한 선택적인 최대 문자 수 (기본값 4000, 최대 20000).",
			},
		},
	}
}

func (t *ShellJobTool) IsReadOnly() bool {
	return true
}

func (t *ShellJobTool) MaxResultSizeChars() int {
	return 100000
}

func (t *ShellJobTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	out := make(chan tool.ToolEvent, 2)
	if ctx.ShellJobs == nil {
		return nil, fmt.Errorf("쉘 작업 관리자를 사용할 수 없습니다")
	}

	var req struct {
		Action    string `json:"action"`
		JobID     string `json:"job_id"`
		TailChars int    `json:"tail_chars"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	go func() {
		defer close(out)
		action := strings.ToLower(strings.TrimSpace(req.Action))
		if action == "" {
			action = "list"
		}

		switch action {
		case "list":
			payload, _ := json.Marshal(map[string]any{
				"jobs": ctx.ShellJobs.List(),
			})
			out <- tool.NewOutputEvent(string(payload))
			out <- tool.NewDoneEvent()
			return
		case "status":
			jobID := strings.TrimSpace(req.JobID)
			if jobID == "" {
				out <- tool.NewErrorEvent(fmt.Errorf("status 액션에는 job_id가 필요합니다"))
				return
			}
			snap, ok := ctx.ShellJobs.Snapshot(jobID)
			if !ok {
				out <- tool.NewErrorEvent(fmt.Errorf("작업을 찾을 수 없습니다: %s", jobID))
				return
			}
			payload, _ := json.Marshal(map[string]any{
				"job": snap,
			})
			out <- tool.NewOutputEvent(string(payload))
			out <- tool.NewDoneEvent()
			return
		case "tail":
			jobID := strings.TrimSpace(req.JobID)
			if jobID == "" {
				out <- tool.NewErrorEvent(fmt.Errorf("tail 액션에는 job_id가 필요합니다"))
				return
			}
			maxChars := req.TailChars
			if maxChars <= 0 {
				maxChars = 4000
			}
			if maxChars > 20000 {
				maxChars = 20000
			}
			tail, err := ctx.ShellJobs.Tail(jobID, maxChars)
			if err != nil {
				out <- tool.NewErrorEvent(err)
				return
			}
			payload, _ := json.Marshal(map[string]any{
				"job_id": jobID,
				"tail":   tail,
			})
			out <- tool.NewOutputEvent(string(payload))
			out <- tool.NewDoneEvent()
			return
		default:
			out <- tool.NewErrorEvent(fmt.Errorf("알 수 없는 액션: %s", action))
			return
		}
	}()

	return out, nil
}

func (t *ShellJobTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	return tool.DefaultAllowPermission(), nil
}
