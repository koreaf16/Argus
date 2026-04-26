// Package agent
// File: loop_proposal.go
// Description: propose_task 도구 결과에서 PendingProposal을 활성화하고 사용자 확인 후 TaskContext를 선언하는 로직
// Responsibility: activateProposalFromToolResults, executePendingProposal 두 함수만 담당

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/agent/taskctx"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/store"
)

// activateProposalFromToolResults는 propose_task 도구 결과에서 PendingProposal을 활성화한다.
func (a *Agent) activateProposalFromToolResults(toolCalls []llm.ToolCall) {
	for _, tc := range toolCalls {
		if tc.Function.Name != "propose_task" {
			continue
		}
		args := parseArgsBestEffort(tc.Function.Arguments)
		if args == nil {
			slog.Warn("propose_task args parse failed")
			continue
		}
		draft := taskctx.TaskContextDraft{}
		draft.Title, _ = args["title"].(string)
		draft.Title = strings.TrimSpace(draft.Title)
		if draft.Title == "" {
			continue
		}
		kindStr, _ := args["kind"].(string)
		draft.Kind = taskctx.TaskKind(strings.TrimSpace(kindStr))
		draft.TargetServer, _ = args["target_server"].(string)
		draft.TargetAccount, _ = args["target_account"].(string)
		draft.WorkingDir, _ = args["working_dir"].(string)
		if rawForbidden, ok := args["forbidden"].([]any); ok {
			for _, item := range rawForbidden {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					draft.Forbidden = append(draft.Forbidden, strings.TrimSpace(s))
				}
			}
		}
		if rawPre, ok := args["preconditions"].([]any); ok {
			for _, item := range rawPre {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					draft.Preconditions = append(draft.Preconditions, strings.TrimSpace(s))
				}
			}
		}
		if rawPlan, ok := args["plan"].([]any); ok {
			for _, item := range rawPlan {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					draft.PlanSteps = append(draft.PlanSteps, strings.TrimSpace(s))
				}
			}
		}
		if rawKF, ok := args["kind_fields"].(map[string]any); ok {
			draft.KindFields = make(map[string]string, len(rawKF))
			for k, v := range rawKF {
				if s, ok := v.(string); ok {
					draft.KindFields[k] = s
				}
			}
		}
		riskAnalysis, _ := args["risk_analysis"].(string)
		reasoning, _ := args["reasoning"].(string)
		var rollbackPlan []string
		if rawRB, ok := args["rollback_plan"].([]any); ok {
			for _, item := range rawRB {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					rollbackPlan = append(rollbackPlan, strings.TrimSpace(s))
				}
			}
		}
		pp := &taskctx.PendingProposal{
			ID:           taskctx.NewID(),
			Draft:        draft,
			RiskAnalysis: strings.TrimSpace(riskAnalysis),
			RollbackPlan: rollbackPlan,
			Reasoning:    strings.TrimSpace(reasoning),
			CreatedAt:    time.Now(),
			TurnIndex:    a.historyTurnCounter,
		}
		a.pendingProposal = pp
		slog.Info("pending proposal activated", "title", draft.Title, "kind", draft.Kind)
		a.notifyTaskProposed(draft.Title, string(draft.Kind), draft.TargetServer, draft.TargetAccount)
		return
	}
}

// executePendingProposal은 사용자가 작업 제안을 확인한 후 TaskContext를 선언하고 응답을 반환한다.
func (a *Agent) executePendingProposal(
	ctx context.Context,
	proposal *taskctx.PendingProposal,
	_ []store.Server,
) error {
	if proposal == nil {
		return nil
	}
	slog.Info("executing pending proposal", "title", proposal.Draft.Title, "kind", proposal.Draft.Kind)

	if a.taskMgr != nil {
		req := proposal.Draft.ToDeclareRequest()
		if tc, err := a.taskMgr.Declare(req); err != nil {
			slog.Warn("pending proposal declare task failed", "err", err)
			a.handler.OnResponse(fmt.Sprintf("작업 선언 실패: %s", err.Error()))
		} else {
			a.notifyTaskDeclared(tc.ID, tc.Title, string(tc.Kind))
			a.handler.OnResponse(fmt.Sprintf(
				"작업이 확정되었습니다: %s (ID: %s)\n지금 첫 번째 단계를 시작하겠습니다.",
				tc.Title, tc.ID,
			))
		}
	} else {
		a.handler.OnResponse(fmt.Sprintf(
			"작업이 확정되었습니다: %s\n지금 첫 번째 단계를 시작하겠습니다.",
			proposal.Draft.Title,
		))
	}
	_ = ctx
	return nil
}
