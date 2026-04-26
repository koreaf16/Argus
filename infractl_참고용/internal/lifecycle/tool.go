// Package lifecycle
// File: tool.go
// Description: 라이프사이클 훅 등록 LLM 도구 구현
// Responsibility: LLM이 자연어로 라이프사이클 훅을 등록할 수 있는 도구 제공

package lifecycle

import (
	"context"
	"fmt"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/tools"
)

// RegisterTool은 라이프사이클 훅을 등록하는 LLM 도구이다.
type RegisterTool struct {
	Manager *Manager
}

func (t *RegisterTool) Name() string { return "hook_register" }
func (t *RegisterTool) Description() string {
	return "라이프사이클 훅을 등록한다. 특정 이벤트 발생 시 스크립트가 자동으로 실행된다."
}
func (t *RegisterTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"event": map[string]interface{}{
				"type":        "string",
				"description": "훅 이벤트 (before_execute|after_execute|on_error|on_connect|on_session_start)",
				"enum": []string{
					"before_execute", "after_execute", "on_error",
					"on_connect", "on_session_start",
				},
			},
			"script_path": map[string]interface{}{
				"type":        "string",
				"description": "실행할 스크립트 절대 경로 (예: /home/user/.infractl/hooks/notify.sh)",
			},
			"condition": map[string]interface{}{
				"type":        "string",
				"description": "매칭 조건 (예: 'tool=shell_exec,server=prod'). 비어 있으면 항상 실행",
			},
		},
		"required": []string{"event", "script_path"},
	}
}
func (t *RegisterTool) IsReadOnly() bool { return false }
func (t *RegisterTool) IsEnabled() bool  { return true }

func (t *RegisterTool) Execute(ctx context.Context, args map[string]interface{}, _ executor.Executor) (tools.ToolOutcome, error) {
	event, _ := args["event"].(string)
	scriptPath, _ := args["script_path"].(string)
	condition, _ := args["condition"].(string)

	if event == "" || scriptPath == "" {
		return tools.ToolOutcome{}, fmt.Errorf("event와 script_path는 필수입니다")
	}

	id, err := t.Manager.Register(ctx, Event(event), condition, scriptPath)
	if err != nil {
		return tools.ToolOutcome{}, fmt.Errorf("훅 등록: %w", err)
	}

	condDisplay := condition
	if condDisplay == "" {
		condDisplay = "(항상 실행)"
	}
	return tools.ToolOutcome{Content: fmt.Sprintf("✓ 훅 #%d 등록 완료\n이벤트: %s\n스크립트: %s\n조건: %s",
		id, event, scriptPath, condDisplay), Success: true}, nil
}
