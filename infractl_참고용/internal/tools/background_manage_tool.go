// Package tools
// File: background_manage_tool.go
// Description: 백그라운드 작업 관리 LLM 도구 (list / result / cancel)
// Responsibility: background.Manager 상태를 LLM이 자연어로 조회·취소할 수 있도록 도구 제공

package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/background"
	"github.com/yourorg/infractl/internal/executor"
)

const defaultWaitSeconds = 25

// BackgroundManageTool은 백그라운드 작업을 관리하는 LLM 도구이다.
type BackgroundManageTool struct {
	Manager *background.Manager
}

func (t *BackgroundManageTool) Name() string { return "background_jobs" }

func (t *BackgroundManageTool) Description() string {
	return "백그라운드 작업을 관리한다. action=list: 전체 목록, action=result: 특정 작업 결과, action=cancel: 작업 취소."
}

func (t *BackgroundManageTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"list", "result", "cancel"},
				"description": "수행할 작업 (list: 목록, result: 결과 조회·대기, cancel: 취소)",
			},
			"job_id": map[string]interface{}{
				"type":        "integer",
				"description": "대상 작업 ID (result, cancel에서 필요)",
			},
			"wait_seconds": map[string]interface{}{
				"type":        "integer",
				"description": "result 조회 시 완료를 기다릴 최대 초 (0~60, 기본 25). 0이면 즉시 반환.",
			},
		},
		"required": []string{"action"},
	}
}

func (t *BackgroundManageTool) IsReadOnly() bool { return true }
func (t *BackgroundManageTool) IsEnabled() bool  { return true }

func (t *BackgroundManageTool) Execute(ctx context.Context, args map[string]interface{}, _ executor.Executor) (ToolOutcome, error) {
	action, _ := args["action"].(string)

	switch action {
	case "list":
		return t.handleList()
	case "result":
		id := argInt(args, "job_id", -1)
		if id < 0 {
			return ToolOutcome{}, fmt.Errorf("job_id 파라미터가 필요합니다")
		}
		waitSec := argInt(args, "wait_seconds", defaultWaitSeconds)
		if waitSec < 0 || waitSec > 60 {
			waitSec = defaultWaitSeconds
		}
		return t.handleResult(ctx, id, time.Duration(waitSec)*time.Second)
	case "cancel":
		id := argInt(args, "job_id", -1)
		if id < 0 {
			return ToolOutcome{}, fmt.Errorf("job_id 파라미터가 필요합니다")
		}
		return t.handleCancel(id)
	default:
		return ToolOutcome{}, fmt.Errorf("알 수 없는 action: %s", action)
	}
}

func (t *BackgroundManageTool) handleList() (ToolOutcome, error) {
	jobs := t.Manager.List()
	if len(jobs) == 0 {
		return ToolOutcome{Content: "실행 중이거나 완료된 백그라운드 작업이 없습니다.", Success: true}, nil
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].ID < jobs[j].ID })

	var sb strings.Builder
	sb.WriteString("백그라운드 작업 목록:\n")
	for _, j := range jobs {
		icon := "⏳"
		switch j.Status {
		case background.StatusCompleted:
			icon = "✓"
		case background.StatusFailed:
			icon = "✗"
		case background.StatusCancelled:
			icon = "⊘"
		}
		sb.WriteString(fmt.Sprintf("  #%d %s [%s] %s\n", j.ID, icon, j.Status, j.Description))
	}
	return ToolOutcome{Content: sb.String(), Success: true}, nil
}

func (t *BackgroundManageTool) handleResult(ctx context.Context, id int, wait time.Duration) (ToolOutcome, error) {
	j, err := t.Manager.Get(id)
	if err != nil {
		return ToolOutcome{}, err
	}

	if j.IsActive() && wait > 0 {
		deadline := time.Now().Add(wait)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
	poll:
		for time.Now().Before(deadline) {
			select {
			case <-ctx.Done():
				break poll
			case <-ticker.C:
				if j, err = t.Manager.Get(id); err != nil || !j.IsActive() {
					break poll
				}
			}
		}
	}

	if j.IsActive() {
		return ToolOutcome{
			Content: fmt.Sprintf(
				"작업 #%d '%s' 는 아직 실행 중입니다. 다른 작업을 진행하거나 잠시 뒤 한 번만 더 확인하세요. 같은 호출을 연속으로 반복하지 마세요.",
				id, j.Description),
			Success: true,
		}, nil
	}
	if j.Error != "" {
		return ToolOutcome{Content: fmt.Sprintf("작업 #%d 실패:\n%s", id, j.Error), Success: true}, nil
	}
	if j.Result == "" {
		return ToolOutcome{Content: fmt.Sprintf("작업 #%d '%s' 완료 (결과 없음)", id, j.Description), Success: true}, nil
	}
	return ToolOutcome{Content: fmt.Sprintf("작업 #%d '%s' 결과:\n%s", id, j.Description, j.Result), Success: true}, nil
}

func (t *BackgroundManageTool) handleCancel(id int) (ToolOutcome, error) {
	if err := t.Manager.Cancel(id); err != nil {
		return ToolOutcome{}, err
	}
	return ToolOutcome{Content: fmt.Sprintf("작업 #%d 취소 완료.", id), Success: true}, nil
}
