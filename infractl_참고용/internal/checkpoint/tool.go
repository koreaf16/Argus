// Package checkpoint
// File: tool.go
// Description: 체크포인트 관리 LLM 도구
// Responsibility: 체크포인트 목록 조회 및 롤백 명령 제공

package checkpoint

import (
	"github.com/yourorg/infractl/internal/tools"
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

// ListTool은 체크포인트 목록을 조회한다.
type ListTool struct {
	Manager *Manager
}

func (t *ListTool) Name() string { return "checkpoint_list" }

func (t *ListTool) Description() string {
	return "특정 서버의 최근 체크포인트 목록을 조회한다. 롤백이 필요할 때 최근 상태를 확인하는 용도."
}

func (t *ListTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"server": map[string]interface{}{
				"type":        "string",
				"description": "대상 서버 이름",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "조회할 개수 (기본 10개)",
				"default":     10,
			},
		},
		"required": []string{"server"},
	}
}

func (t *ListTool) IsReadOnly() bool { return true }
func (t *ListTool) IsEnabled() bool  { return true }

func (t *ListTool) Execute(ctx context.Context, args map[string]interface{}, _ executor.Executor) (tools.ToolOutcome, error) {
	server, _ := args["server"].(string)
	limit := 10
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	cps, err := t.Manager.List(ctx, server, limit)
	if err != nil {
		return tools.ToolOutcome{}, fmt.Errorf("list checkpoints: %w", err)
	}

	if len(cps) == 0 {
		return tools.ToolOutcome{Content: "체크포인트가 없습니다.", Success: true}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[%s] 최근 체크포인트 목록:\n", server))
	for _, cp := range cps {
		sb.WriteString(fmt.Sprintf("  #%d [%s] %s (%s)\n", cp.ID, cp.CreatedAt.Format("2006-01-02 15:04:05"), cp.Description, cp.ToolName))
	}
	return tools.ToolOutcome{Content: sb.String(), Success: true}, nil
}

// RollbackTool은 체크포인트를 기반으로 롤백 명령을 생성/실행한다.
type RollbackTool struct {
	Manager *Manager
}

func (t *RollbackTool) Name() string { return "checkpoint_rollback" }

func (t *RollbackTool) Description() string {
	return "특정 체크포인트의 롤백 명령을 서버에서 실행한다. 실수로 데이터를 변경하거나 삭제했을 때 복구용으로 사용."
}

func (t *RollbackTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"checkpoint_id": map[string]interface{}{
				"type":        "integer",
				"description": "롤백할 체크포인트 ID",
			},
		},
		"required": []string{"checkpoint_id"},
	}
}

func (t *RollbackTool) IsReadOnly() bool { return false }
func (t *RollbackTool) IsEnabled() bool  { return true }

func (t *RollbackTool) Execute(ctx context.Context, args map[string]interface{}, exec executor.Executor) (tools.ToolOutcome, error) {
	cpIDRaw := args["checkpoint_id"]
	var cpID int64
	switch v := cpIDRaw.(type) {
	case float64:
		cpID = int64(v)
	case int64:
		cpID = v
	default:
		return tools.ToolOutcome{}, fmt.Errorf("checkpoint_id must be an integer")
	}

	cp, err := t.Manager.Get(ctx, cpID)
	if err != nil {
		return tools.ToolOutcome{}, fmt.Errorf("get checkpoint: %w", err)
	}
	if exec != nil {
		target := strings.TrimSpace(exec.Target())
		if target != "" && !executor.IsLocalTarget(target) &&
			cp.Server != "" && !strings.EqualFold(target, cp.Server) {
			return tools.ToolOutcome{}, fmt.Errorf("executor target mismatch: checkpoint server=%s executor target=%s", cp.Server, target)
		}
	}

	cmd := t.Manager.BuildRollbackCommand(cp)
	if cmd == "" {
		return tools.ToolOutcome{Content: "이 체크포인트에는 저장된 롤백 명령이 없습니다.", Success: true}, nil
	}

	res, err := exec.Execute(ctx, cmd)
	if err != nil {
		return tools.ToolOutcome{}, fmt.Errorf("rollback execution failed: %w", err)
	}

	return tools.ToolOutcome{Content: fmt.Sprintf("✓ 롤백 성공 (CP #%d)\n명령: %s\n출력:\n%s", cpID, cmd, res.Stdout), Success: true}, nil
}
