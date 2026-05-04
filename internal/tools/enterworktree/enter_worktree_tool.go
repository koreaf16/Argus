package enterworktree

import (
	"encoding/json"
	"github.com/koreaf16/argus/internal/tools"
)

type EnterWorktreeTool struct{}

func NewEnterWorktreeTool() *EnterWorktreeTool { return &EnterWorktreeTool{} }
func (t *EnterWorktreeTool) Name() string      { return "enter_worktree" }
func (t *EnterWorktreeTool) Description(ctx tools.Context) string {
	return "새로운 작업 트리(Worktree) 환경으로 전환합니다."
}
func (t *EnterWorktreeTool) InputSchema() tools.ToolInputJSONSchema {
	return tools.ToolInputJSONSchema{
		"type":       "object",
		"properties": map[string]interface{}{"worktree_id": map[string]interface{}{"type": "string"}},
		"required":   []string{"worktree_id"},
	}
}
func (t *EnterWorktreeTool) IsReadOnly() bool { return false }
func (t *EnterWorktreeTool) Call(ctx tools.Context, input json.RawMessage) (<-chan tools.ToolEvent, error) {
	out := make(chan tools.ToolEvent, 1)
	out <- tools.NewOutputEvent("작업 트리 전환 완료")
	close(out)
	return out, nil
}
func (t *EnterWorktreeTool) CheckPermission(ctx tools.Context, input json.RawMessage) (tools.PermissionResult, error) {
	return tools.DefaultAllowPermission(), nil
}
func (t *EnterWorktreeTool) MaxResultSizeChars() int { return 1000 }
