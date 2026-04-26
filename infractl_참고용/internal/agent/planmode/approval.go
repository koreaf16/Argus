// Package planmode
// File: approval.go
// Description: Plan Mode 종료 시 보류 명령 승인 UI 트리거 + 직접 재실행 오케스트레이션
// Responsibility: ApprovalHandler interface, Trigger 함수, 승인된 명령 직접 실행

// Ported from: claude_cli/src/tools/ExitPlanModeTool/ExitPlanModeV2Tool.ts:243-403
// 핵심 패턴: ExitPlanMode 승인 → 계획을 LLM 재호출 없이 직접 실행.
//            ApprovalHandler 는 TUI/CLI 어댑터 인터페이스로 추상화한다.

package planmode

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/tools"
)

// ApprovalHandler 는 승인 UI 를 제공하는 어댑터 인터페이스다.
// TUI 모달 또는 CLI stdin 두 구현체가 이 인터페이스를 따른다.
type ApprovalHandler interface {
	// ShowApproval 은 pending 명령 목록을 사용자에게 보여주고 승인된 항목 ID 를 반환한다.
	// 빈 슬라이스를 반환하면 모두 거부로 처리한다. 취소(Esc) 시 nil, true 를 반환한다.
	ShowApproval(ctx context.Context, cmds []PendingCommand) (approvedIDs []string, cancelled bool)
}

// ApprovalResult 는 Trigger 의 실행 결과를 담는다.
type ApprovalResult struct {
	Approved []ExecutedCommand // 승인 후 실행된 명령
	Rejected []PendingCommand  // 거부된 명령
}

// ExecutedCommand 는 승인 + 실행된 명령과 결과를 담는다.
type ExecutedCommand struct {
	PendingCommand
	Output  string
	IsError bool
}

// ToolResultAppender 는 실행 결과를 세션에 append 하는 콜백이다.
// agent.Agent 의 메서드를 주입해 Plan Mode 재실행 결과가 다음 LLM turn 에 반영되게 한다.
type ToolResultAppender func(toolID, result string)

// Trigger 는 Plan Mode Exit 시 승인 UI 를 열고, 승인된 명령을 직접 실행한다.
// cmds 가 비어있으면 UI 없이 즉시 빈 ApprovalResult 를 반환한다.
func Trigger(
	ctx context.Context,
	cmds []PendingCommand,
	registry *tools.Registry,
	exec executor.Executor,
	handler ApprovalHandler,
	appender ToolResultAppender,
) (ApprovalResult, error) {
	if len(cmds) == 0 {
		return ApprovalResult{}, nil
	}

	approvedIDs, cancelled := handler.ShowApproval(ctx, cmds)
	if cancelled {
		slog.Info("plan mode approval cancelled — all commands rejected",
			"count", len(cmds))
		return ApprovalResult{Rejected: cmds}, nil
	}

	approved := make(map[string]bool, len(approvedIDs))
	for _, id := range approvedIDs {
		approved[id] = true
	}

	result := ApprovalResult{}
	for _, cmd := range cmds {
		if !approved[cmd.ID] {
			result.Rejected = append(result.Rejected, cmd)
			continue
		}

		out, isErr := executeTool(ctx, cmd, registry, exec)
		ec := ExecutedCommand{PendingCommand: cmd, Output: out, IsError: isErr}
		result.Approved = append(result.Approved, ec)

		slog.Info("plan mode approved command executed",
			"tool", cmd.Tool,
			"id", cmd.ID,
			"is_error", isErr,
		)

		if appender != nil {
			appender(cmd.ID, out)
		}
	}

	return result, nil
}

// executeTool 은 레지스트리에서 도구를 꺼내 args 로 직접 실행한다.
func executeTool(ctx context.Context, cmd PendingCommand, registry *tools.Registry, exec executor.Executor) (string, bool) {
	if registry == nil {
		return "Error: tool registry not available", true
	}

	t, ok := registry.Get(cmd.Tool)
	if !ok {
		return fmt.Sprintf("Error: tool %q not found in registry", cmd.Tool), true
	}

	args := cmd.Args
	if args == nil {
		args = map[string]any{}
	}

	outcome, err := t.Execute(ctx, args, exec)
	if err != nil {
		return fmt.Sprintf("Error: %s", err.Error()), true
	}
	if !outcome.Success {
		msg := outcome.ErrorMessage
		if msg == "" {
			msg = outcome.Content
		}
		return msg, true
	}
	return outcome.Content, false
}

// MarshalArgs 는 args map 을 JSON 문자열로 직렬화한다.
// tool_invoker 가 세션 로그에 추가할 때 사용한다.
func MarshalArgs(args map[string]any) string {
	b, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(b)
}
