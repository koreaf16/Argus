// Package agent
// File: loop_pending.go
// Description: 사용자가 제안된 명령을 승인("예/yes")했을 때의 즉시 실행 분기
// Responsibility: 분류·intel 단계 우회, CONFIRMED ACTION 힌트 주입, 단일 명령만 처리

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/store"
)

// maxPendingToolLoop는 pending action 실행 시 최대 도구 호출 횟수다.
// 단일 명령 실행에 집중하므로 일반 루프(50)보다 작게 설정한다.
const maxPendingToolLoop = 10

// executePendingAction은 사용자가 제안된 명령을 확인한 후 호출된다.
// 분류·intel 단계를 건너뛰고 직접 LLM에 CONFIRMED ACTION 힌트를 주입하여 실행한다.
func (a *Agent) executePendingAction(
	ctx context.Context,
	pending *PendingAction,
	servers []store.Server,
) error {
	if pending == nil {
		return nil
	}

	slog.Info("executing pending action",
		"command", truncatePendingPreview(pending.ProposedCommand, 100),
		"source", pending.Source,
		"age_ms", time.Since(pending.CreatedAt).Milliseconds())

	// connStates는 서명에서 제외했으므로 내부에서 직접 수집한다
	var connStates []connector.ConnectorState
	if a.connectorMgr != nil {
		states := a.connectorMgr.States()
		if a.activeServer == nil {
			connStates = states
		} else {
			for _, st := range states {
				if strings.EqualFold(st.ServerName, a.activeServer.Name) {
					connStates = append(connStates, st)
				}
			}
		}
	}

	now := time.Now()
	infractlMD := a.promptInputs.GetInfractlMD(loadInfractlMDData)

	// 도구 목록: shell + file_ops + system_info 핵심 그룹 + propose_action
	allowedTools := ResolveToolGroups(
		[]string{"shell", "file_ops", "system_info", "connector"},
		a.registry,
		a.connectorMgr,
	)
	allowedTools["propose_action"] = true
	enabledTools := a.registry.GetEnabledFiltered(allowedTools)
	toolDefs := a.registry.ToToolDefsFiltered(allowedTools)

	// 시스템 프롬프트: 간소화된 섹션 집합 + CONFIRMED ACTION 힌트
	sections := ResolveSectionsFromList(
		[]string{"safety", "tool_priority", "tool_selection"},
		true,
		a.activeServer != nil,
		len(servers) > 0,
		len(connStates) > 0,
		false,
		infractlMD != "",
	)

	layout := BuildContextualLayoutAt(
		sections,
		enabledTools,
		infractlMD,
		servers,
		a.activeServer,
		connStates,
		nil, // learnedSystems — 불필요
		nil, // ragSources — 불필요
		nil, // knowledgeStats — 불필요
		a.modelName,
		now,
		"",
	)
	systemPrompt := layout.Render("", "")
	systemPrompt += buildConfirmedActionHint(pending)

	systemMsg := llm.Message{Role: llm.RoleSystem, Content: systemPrompt}

	// 기존 최대 도구 루프를 임시 제한
	origMaxToolLoop := a.maxToolLoop
	a.maxToolLoop = maxPendingToolLoop
	defer func() { a.maxToolLoop = origMaxToolLoop }()

	activeClient, activeTier, activeModelName := a.resolveClientForTier("general")
	llm.LogSystemEvent("Pending Action Execution", fmt.Sprintf(
		"Command: %s\nPlan steps: %d\nModel: %s",
		truncatePendingPreview(pending.ProposedCommand, 120),
		len(pending.Plan),
		activeModelName,
	))

	apiTools := toolDefs
	if strings.Contains(strings.ToLower(activeModelName), "qwen") {
		apiTools = nil
	}

	return a.runWithEngine(ctx, systemMsg, activeClient, activeTier, activeModelName, apiTools, false)
}

// activatePendingFromToolResults는 도구 호출 결과에서 propose_action MetadataJSON을 감지하고
// PendingActionTracker를 활성화한다. LLM 루프에서 매 도구 실행 후 호출된다.
func (a *Agent) activatePendingFromToolResults(toolCalls []llm.ToolCall, toolResults []llm.Message) {
	for i, tc := range toolCalls {
		if tc.Function.Name != "propose_action" {
			continue
		}
		if i >= len(toolResults) {
			continue
		}
		// 도구 결과 메시지의 Content에서 MetadataJSON 대신 tool_exec의 MetadataJSON을 직접 접근 불가.
		// propose_action 도구 실행 결과에는 MetadataJSON이 ToolOutcome에 있지만,
		// llm.Message로 변환된 toolResults에는 Content만 있다.
		// 대신 propose_action의 Content가 표준 포맷이므로 args에서 직접 파싱한다.
		args := parseArgsBestEffort(tc.Function.Arguments)
		if args == nil {
			slog.Warn("propose_action args parse failed")
			continue
		}
		command, _ := args["command"].(string)
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		reason, _ := args["reason"].(string)
		var plan []string
		if rawPlan, ok := args["plan"].([]any); ok {
			for _, item := range rawPlan {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					plan = append(plan, strings.TrimSpace(s))
				}
			}
		}
		serverName := ""
		if a.activeServer != nil {
			serverName = a.activeServer.Name
		}
		pa := &PendingAction{
			ProposedCommand: command,
			Reason:          strings.TrimSpace(reason),
			Plan:            plan,
			ServerName:      serverName,
			Source:          "tool",
			CreatedAt:       time.Now(),
		}
		a.pendingAction.Activate(pa, a.historyTurnCounter)
		if len(plan) > 0 {
			a.currentPlan = NewPlanState(plan)
			slog.Debug("plan state initialized", "steps", len(plan))
		}
		return
	}
}

// buildConfirmedActionHint는 LLM에게 사용자가 이미 승인한 명령을 즉시 실행하도록 지시하는 프롬프트를 반환한다.
func buildConfirmedActionHint(pending *PendingAction) string {
	var sb strings.Builder

	sb.WriteString("\n\n## [CONFIRMED ACTION]\n")
	sb.WriteString("사용자가 직전에 제안된 명령을 승인했습니다. ")
	sb.WriteString("아래 명령을 **즉시** 실행하세요.\n\n")
	sb.WriteString("**엄격히 준수:**\n")
	sb.WriteString("- 추가 질문/탐색/환경 조사 금지\n")
	sb.WriteString("- 다른 문제(패키지 의존성, 버전 등)를 먼저 해결하려 하지 말 것\n")
	sb.WriteString("- 이 명령 하나를 실행하고 결과를 보고하라\n\n")

	sb.WriteString("**실행할 명령:**\n```bash\n")
	sb.WriteString(pending.ProposedCommand)
	sb.WriteString("\n```\n")

	if pending.Reason != "" {
		sb.WriteString("\n**배경:** ")
		sb.WriteString(pending.Reason)
		sb.WriteString("\n")
	}

	if len(pending.Plan) > 0 {
		sb.WriteString("\n**이후 단계 (이번에는 실행하지 않음):**\n")
		for i, step := range pending.Plan {
			fmt.Fprintf(&sb, "%d. %s\n", i+1, step)
		}
	}

	sb.WriteString("\n명령 실행 후 결과를 사용자에게 보고하세요. ")
	sb.WriteString("성공하면 다음 단계 진행 여부를 propose_action으로 제안하세요.\n")

	return sb.String()
}
