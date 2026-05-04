package exitworktree

import (
	"encoding/json"
	"github.com/koreaf16/argus/internal/tools"
)

type ExitWorktreeTool struct{}

func NewExitWorktreeTool() *ExitWorktreeTool { return &ExitWorktreeTool{} }
func (t *ExitWorktreeTool) Name() string     { return "exit_worktree" }
func (t *ExitWorktreeTool) Description(ctx tools.Context) string {
	return "현재 작업 트리 환경을 종료합니다."
}
func (t *ExitWorktreeTool) InputSchema() tools.ToolInputJSONSchema {
	return tools.ToolInputJSONSchema{"type": "object"}
}
func (t *ExitWorktreeTool) IsReadOnly() bool { return false }
func (t *ExitWorktreeTool) Call(ctx tools.Context, input json.RawMessage) (<-chan tools.ToolEvent, error) {
	out := make(chan tools.ToolEvent, 1)
	out <- tools.NewOutputEvent("작업 트리 종료 완료")
	close(out)
	return out, nil
}
func (t *ExitWorktreeTool) CheckPermission(ctx tools.Context, input json.RawMessage) (tools.PermissionResult, error) {
	return tools.DefaultAllowPermission(), nil
}
func (t *ExitWorktreeTool) MaxResultSizeChars() int { return 1000 }
