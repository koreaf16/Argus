// Package tools
// File: structured_output_tool.go
// Description: LLM의 구조화된 답변(JSON)을 받기 위한 가상 도구
// Responsibility: 지정된 JSON 스키마를 따르는 답변을 받아 엔진에 전달

package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yourorg/infractl/internal/executor"
)

const StructuredOutputToolName = "structured_output_tool"

// StructuredOutputTool 은 LLM이 특정 스키마를 준수하여 최종 답변을 제출하도록 강제할 때 사용된다.
type StructuredOutputTool struct {
	Schema map[string]interface{}
}

func (t *StructuredOutputTool) Name() string { return StructuredOutputToolName }

func (t *StructuredOutputTool) Description() string {
	return "Use this tool to submit your final response in a structured JSON format. " +
		"Ensure your arguments exactly match the provided schema."
}

func (t *StructuredOutputTool) IsReadOnly() bool { return true }
func (t *StructuredOutputTool) IsEnabled() bool  { return true }

func (t *StructuredOutputTool) Parameters() map[string]interface{} {
	if t.Schema == nil {
		return map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}
	}
	return t.Schema
}

func (t *StructuredOutputTool) Execute(ctx context.Context, args map[string]interface{}, _ executor.Executor) (ToolOutcome, error) {
	// 실제 실행은 하지 않고, 인자로 들어온 JSON 데이터를 문자열로 변환하여 결과로 반환한다.
	// 엔진 레이어에서 이 도구의 호출 여부와 인자 값을 확인하게 된다.
	data, err := json.MarshalIndent(args, "", "  ")
	if err != nil {
		return ToolOutcome{
			Content:      fmt.Sprintf("Error formatting structured output: %v", err),
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return ToolOutcome{
		Content: string(data),
		Success: true,
	}, nil
}
