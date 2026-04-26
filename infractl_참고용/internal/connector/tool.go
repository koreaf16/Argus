// Package connector
// File: tool.go
// Description: 커넥터 생성 도구의 공통 래퍼 구조체
// Responsibility: ConnectorStatus 포인터와 연동된 IsEnabled()를 제공하는 범용 도구 래퍼

package connector

import (
	"context"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/tools"
)

// ConnectorTool은 커넥터가 생성하는 도구의 공통 구현체이다.
// status 포인터를 통해 커넥터 상태와 IsEnabled()가 연동된다.
type ConnectorTool struct {
	name        string
	description string
	params      map[string]interface{}
	readOnly    bool
	status      *ConnectorStatus
	OutputCb    func(string)
	executeFn   func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (string, error)
}

// NewConnectorTool은 ConnectorTool을 생성한다.
// status 포인터를 부모 커넥터로부터 전달받아 IsEnabled()를 연동한다.
func NewConnectorTool(
	name, description string,
	params map[string]interface{},
	readOnly bool,
	status *ConnectorStatus,
	executeFn func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (string, error),
) *ConnectorTool {
	return &ConnectorTool{
		name:        name,
		description: description,
		params:      params,
		readOnly:    readOnly,
		status:      status,
		executeFn:   executeFn,
	}
}

func (t *ConnectorTool) Name() string                       { return t.name }
func (t *ConnectorTool) Description() string                { return t.description }
func (t *ConnectorTool) Parameters() map[string]interface{} { return t.params }
func (t *ConnectorTool) IsReadOnly() bool                   { return t.readOnly }

// IsEnabled는 커넥터가 connected 상태일 때만 true를 반환한다.
func (t *ConnectorTool) IsEnabled() bool {
	if t.status == nil {
		return false
	}
	return *t.status == StatusConnected
}

// SetOutputCb는 실시간 출력을 위한 콜백을 설정한다.
func (t *ConnectorTool) SetOutputCb(cb func(string)) {
	t.OutputCb = cb
}

// Execute는 executeFn을 호출하고 결과를 ToolOutcome으로 감싼다.
func (t *ConnectorTool) Execute(ctx context.Context, args map[string]interface{}, exec executor.Executor) (tools.ToolOutcome, error) {
	// executeFn 내부에서 t.OutputCb 등에 접근할 수 있도록 컨텍스트에 주입
	ctx = context.WithValue(ctx, "tool", t)
	result, err := t.executeFn(ctx, args, exec)
	if err != nil {
		return tools.ToolOutcome{}, err
	}
	return tools.ToolOutcome{Content: result, Success: true}, nil
}

// targetParam은 도구 파라미터에 target 필드를 추가하는 헬퍼이다.
func targetParam() map[string]interface{} {
	return map[string]interface{}{
		"type":        "string",
		"description": "대상 서버 이름. 커넥터가 등록된 서버를 자동으로 사용하려면 생략.",
	}
}
