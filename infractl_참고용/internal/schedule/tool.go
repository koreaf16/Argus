// Package schedule
// File: tool.go
// Description: 작업 스케줄링 LLM 도구
// Responsibility: 크론 표현식을 사용한 주기적 작업(프롬프트) 등록/조회

package schedule

import (
	"github.com/yourorg/infractl/internal/tools"
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/store"
)

// CreateTool은 새 스케줄을 등록한다.
type CreateTool struct {
	Scheduler *Scheduler
}

func (t *CreateTool) Name() string { return "schedule_create" }

func (t *CreateTool) Description() string {
	return "프롬프트를 주기적으로 실행하도록 스케줄을 등록한다. (예: 매일 오전 9시에 DB 상태 점검)"
}

func (t *CreateTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "스케줄 이름",
			},
			"cron_expr": map[string]interface{}{
				"type":        "string",
				"description": "크론 표현식 (예: '0 9 * * *')",
			},
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "실행할 프롬프트 내용",
			},
		},
		"required": []string{"name", "cron_expr", "prompt"},
	}
}

func (t *CreateTool) IsReadOnly() bool { return false }
func (t *CreateTool) IsEnabled() bool  { return true }

func (t *CreateTool) Execute(ctx context.Context, args map[string]interface{}, _ executor.Executor) (tools.ToolOutcome, error) {
	name, _ := args["name"].(string)
	cronExpr, _ := args["cron_expr"].(string)
	prompt, _ := args["prompt"].(string)

	_, err := t.Scheduler.Add(ctx, store.Schedule{
		Name:     name,
		CronExpr: cronExpr,
		Prompt:   prompt,
		Enabled:  true,
	})
	if err != nil {
		return tools.ToolOutcome{}, fmt.Errorf("create schedule: %w", err)
	}
	return tools.ToolOutcome{Content: fmt.Sprintf("스케줄 '%s'가 등록되었습니다: %s", name, cronExpr), Success: true}, nil
}

// ListTool은 스케줄 목록을 조회한다.
type ListTool struct {
	Scheduler *Scheduler
}

func (t *ListTool) Name() string { return "schedule_list" }

func (t *ListTool) Description() string {
	return "등록된 모든 작업 스케줄 목록을 조회한다."
}

func (t *ListTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}

func (t *ListTool) IsReadOnly() bool { return true }
func (t *ListTool) IsEnabled() bool  { return true }

func (t *ListTool) Execute(ctx context.Context, _ map[string]interface{}, _ executor.Executor) (tools.ToolOutcome, error) {
	schedules, err := t.Scheduler.List(ctx)
	if err != nil {
		return tools.ToolOutcome{}, fmt.Errorf("list schedules: %w", err)
	}
	if len(schedules) == 0 {
		return tools.ToolOutcome{Content: "등록된 스케줄이 없습니다.", Success: true}, nil
	}

	var sb strings.Builder
	sb.WriteString("등록된 스케줄 목록:\n")
	for _, s := range schedules {
		sb.WriteString(fmt.Sprintf("  - %s: %s (Prompt: %s)\n", s.Name, s.CronExpr, s.Prompt))
	}
	return tools.ToolOutcome{Content: sb.String(), Success: true}, nil
}
