// Package connector
// File: generated_tool.go
// Description: 자동 생성된 커넥터 도구 래퍼
// Responsibility: LLM이 생성하거나 가공한 도구를 커넥터 상태와 연동하여 래핑

package connector

import (
	"context"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/tools"
)

// GeneratedTool은 커넥터에서 생성된 도구를 래핑한다.
type GeneratedTool struct {
	name        string
	description string
	params      map[string]interface{}
	readOnly    bool
	status      *ConnectorStatus
	executeFn   func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (string, error)
}

// NewGeneratedTool은 GeneratedTool을 생성한다.
func NewGeneratedTool(
	name, description string,
	params map[string]interface{},
	readOnly bool,
	status *ConnectorStatus,
	executeFn func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (string, error),
) *GeneratedTool {
	return &GeneratedTool{
		name:        name,
		description: description,
		params:      params,
		readOnly:    readOnly,
		status:      status,
		executeFn:   executeFn,
	}
}

func (t *GeneratedTool) Name() string                       { return t.name }
func (t *GeneratedTool) Description() string                { return t.description }
func (t *GeneratedTool) Parameters() map[string]interface{} { return t.params }
func (t *GeneratedTool) IsReadOnly() bool                   { return t.readOnly }

func (t *GeneratedTool) IsEnabled() bool {
	if t.status == nil {
		return false
	}
	return *t.status == StatusConnected
}

func (t *GeneratedTool) Execute(ctx context.Context, args map[string]interface{}, exec executor.Executor) (tools.ToolOutcome, error) {
	result, err := t.executeFn(ctx, args, exec)
	if err != nil {
		return tools.ToolOutcome{}, err
	}
	return tools.ToolOutcome{Content: result, Success: true}, nil
}
