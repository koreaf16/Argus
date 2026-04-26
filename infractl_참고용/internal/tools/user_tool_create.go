// Package tools
// File: user_tool_create.go
// Description: 사용자 정의 도구 생성 LLM 도구
// Responsibility: LLM이 user_tool_create를 호출하여 스크립트 기반 커스텀 도구를 등록

package tools

import (
	"context"
	"fmt"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/store"
)

// UserToolCreateTool은 LLM이 사용자 정의 스크립트 도구를 생성하는 도구이다.
type UserToolCreateTool struct {
	Manager *UserToolManager
}

func (t *UserToolCreateTool) Name() string     { return "user_tool_create" }
func (t *UserToolCreateTool) IsReadOnly() bool { return false }
func (t *UserToolCreateTool) IsEnabled() bool  { return t.Manager != nil }

func (t *UserToolCreateTool) Description() string {
	return "Create a custom reusable tool from a bash script. " +
		"Use when the user asks to create a custom automation tool or shortcut command. " +
		"The tool is saved and registered immediately for future use in the same session."
}

func (t *UserToolCreateTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Tool name: lowercase letters, numbers, underscores only (e.g. 'cleanup_archive')",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "What this tool does — shown to LLM for future tool selection",
			},
			"script_content": map[string]interface{}{
				"type":        "string",
				"description": "Bash script content. Read INFRACTL_ARGS_B64 env var for base64-encoded JSON args.",
			},
			"parameters": map[string]interface{}{
				"type":        "string",
				"description": "Optional JSON Schema for tool parameters",
			},
		},
		"required": []string{"name", "description", "script_content"},
	}
}

func (t *UserToolCreateTool) Execute(ctx context.Context, args map[string]interface{}, _ executor.Executor) (ToolOutcome, error) {
	name, err := argString(args, "name", true)
	if err != nil {
		return ToolOutcome{}, err
	}
	desc, err := argString(args, "description", true)
	if err != nil {
		return ToolOutcome{}, err
	}
	scriptContent, err := argString(args, "script_content", true)
	if err != nil {
		return ToolOutcome{}, err
	}
	parameters, _ := argString(args, "parameters", false)
	if parameters == "" {
		parameters = "{}"
	}

	entry := store.UserToolEntry{
		Name:        name,
		Description: desc,
		Parameters:  parameters,
	}

	id, err := t.Manager.CreateTool(ctx, entry, scriptContent)
	if err != nil {
		return ToolOutcome{}, fmt.Errorf("create tool: %w", err)
	}

	return ToolOutcome{Content: fmt.Sprintf("✓ 도구 '%s' 생성 완료 (ID: %d)\n설명: %s\n\n이제 이 도구를 바로 사용할 수 있습니다.",
		name, id, desc), Success: true}, nil
}
