package snitptool

import (
	"encoding/json"
	"fmt"

	tool "github.com/koreaf16/argus/internal/tools"
)

type SnipTool struct{}

func (t *SnipTool) Name() string {
	return "snip"
}

func (t *SnipTool) Description(ctx tool.Context) string {
	return "요청한 경우 대화 히스토리의 일부를 요약하거나 제거하여 컨텍스트 창을 확보합니다. 아주 긴 대화에서 효율성을 위해 사용됩니다."
}

func (t *SnipTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"reason": map[string]any{
				"type":        "string",
				"description": "히스토리를 정리하려는 이유",
			},
		},
		"required": []string{"reason"},
	}
}

func (t *SnipTool) IsReadOnly() bool {
	return true
}

func (t *SnipTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	out := make(chan tool.ToolEvent, 1)
	defer close(out)

	var in struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		out <- tool.NewErrorEvent(err)
		return out, nil
	}

	out <- tool.NewOutputEvent(fmt.Sprintf("히스토리 정리 요청 수락됨: %s. 다음 턴부터 컨텍스트 최적화가 적용됩니다.", in.Reason))
	return out, nil
}

func (t *SnipTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	return tool.DefaultAllowPermission(), nil
}

func (t *SnipTool) MaxResultSizeChars() int {
	return 1000
}
