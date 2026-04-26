// Package agent
// File: loop_pending_test.go
// Description: Oracle 설치 시나리오 재현 — PendingAction 확인 흐름 통합 테스트
// Responsibility: "예" 응답이 CONFIRMED ACTION 경로를 정확히 트리거함을 검증

package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/tools"
)

// TestBuildConfirmedActionHint_ContainsCommandAndPlan은 힌트 생성기가
// 명령어, 이유, 계획을 모두 포함하는지 검증한다.
func TestBuildConfirmedActionHint_ContainsCommandAndPlan(t *testing.T) {
	pa := &PendingAction{
		ProposedCommand: "sudo -u oracle unzip /home/oracle/LINUX.X64_193000_db_home.zip -d /app/oracle/product/19c/dbhome_1",
		Reason:          "oracle 사용자 권한으로 압축 해제",
		Plan:            []string{"DB Home 압축 해제", "OPatch 교체", "Installer 실행"},
	}
	hint := buildConfirmedActionHint(pa)

	if !strings.Contains(hint, "[CONFIRMED ACTION]") {
		t.Error("hint missing [CONFIRMED ACTION]")
	}
	if !strings.Contains(hint, "sudo -u oracle unzip") {
		t.Error("hint missing proposed command")
	}
	if !strings.Contains(hint, "oracle 사용자 권한으로 압축 해제") {
		t.Error("hint missing reason")
	}
	if !strings.Contains(hint, "DB Home 압축 해제") {
		t.Error("hint missing plan step")
	}
	if !strings.Contains(hint, "추가 질문/탐색/환경 조사 금지") {
		t.Error("hint missing no-exploration directive")
	}
}

// TestActivatePendingFromToolResults_ParsesProposalArgs는 propose_action 도구 호출
// 결과에서 PendingAction이 정확히 활성화되는지 검증한다.
func TestActivatePendingFromToolResults_ParsesProposalArgs(t *testing.T) {
	a := &Agent{
		pendingAction:       newPendingActionTracker(),
		historyTurnCounter:  5,
	}

	args := `{"command":"sudo unzip /tmp/oracle.zip -d /opt/","reason":"권한 문제","plan":["단계1","단계2"]}`
	toolCalls := []llm.ToolCall{{
		ID:       "tc1",
		Function: llm.FunctionCall{Name: "propose_action", Arguments: args},
	}}
	toolResults := []llm.Message{{Role: llm.RoleTool, Content: "계속 진행하시겠습니까?"}}

	a.activatePendingFromToolResults(toolCalls, toolResults)

	pending := a.pendingAction.Current()
	if pending == nil {
		t.Fatal("expected pending to be activated after propose_action tool call")
	}
	if pending.ProposedCommand != "sudo unzip /tmp/oracle.zip -d /opt/" {
		t.Errorf("command = %q", pending.ProposedCommand)
	}
	if pending.Reason != "권한 문제" {
		t.Errorf("reason = %q", pending.Reason)
	}
	if len(pending.Plan) != 2 {
		t.Errorf("plan len = %d, want 2", len(pending.Plan))
	}
	if pending.TurnIndex != 5 {
		t.Errorf("TurnIndex = %d, want 5", pending.TurnIndex)
	}
	if pending.Source != "tool" {
		t.Errorf("source = %q, want tool", pending.Source)
	}
}

// TestActivatePendingFromToolResults_IgnoresNonProposeTools는 propose_action 외의
// 도구 호출은 pending을 활성화하지 않음을 검증한다.
func TestActivatePendingFromToolResults_IgnoresNonProposeTools(t *testing.T) {
	a := &Agent{pendingAction: newPendingActionTracker()}

	toolCalls := []llm.ToolCall{{
		Function: llm.FunctionCall{Name: "shell_exec", Arguments: `{"command":"ls -la"}`},
	}}
	toolResults := []llm.Message{{Role: llm.RoleTool, Content: "output"}}

	a.activatePendingFromToolResults(toolCalls, toolResults)

	if a.pendingAction.Current() != nil {
		t.Error("non-propose_action tool should not activate pending")
	}
}

// TestExecutePendingAction_OracleScenario는 Oracle 19c 설치 중 unzip 제안 후
// 사용자가 "예"를 입력했을 때의 실행 경로를 검증한다.
// 핵심 검증: [CONFIRMED ACTION] 힌트가 LLM 시스템 프롬프트에 주입되어야 한다.
func TestExecutePendingAction_OracleScenario(t *testing.T) {
	execClient := &sequenceLLMClient{
		responses: []llm.Response{
			{Content: "sudo unzip 명령을 실행했습니다. 성공적으로 완료되었습니다."},
		},
	}

	reg := tools.NewRegistry()
	if err := reg.Register(&tools.ProposeActionTool{}); err != nil {
		t.Fatalf("register propose_action: %v", err)
	}

	handler := &captureResponseHandler{}
	a := New(execClient, reg, executor.NewManager(noopAgentExecutor{}), handler, nil)

	pa := &PendingAction{
		ProposedCommand: "sudo -u oracle unzip /home/oracle/LINUX.X64_193000_db_home.zip -d /app/oracle/product/19c/dbhome_1",
		Reason:          "sandbox 권한 실패로 oracle 사용자로 재시도",
		Plan:            []string{"DB Home 압축 해제", "OPatch 교체", "Release Update 적용", "Installer 실행"},
		Source:          "tool",
		CreatedAt:       time.Now(),
	}

	err := a.executePendingAction(context.Background(), pa, nil)
	if err != nil {
		t.Fatalf("executePendingAction() error = %v", err)
	}

	if len(execClient.calls) == 0 {
		t.Fatal("expected LLM call in executePendingAction, got 0")
	}

	foundHint := false
	foundCommand := false
	for _, msgs := range execClient.calls {
		for _, msg := range msgs {
			if msg.Role != llm.RoleSystem {
				continue
			}
			if strings.Contains(msg.Content, "[CONFIRMED ACTION]") {
				foundHint = true
			}
			if strings.Contains(msg.Content, "sudo -u oracle unzip") {
				foundCommand = true
			}
		}
	}
	if !foundHint {
		t.Error("system prompt missing [CONFIRMED ACTION] hint — confirmation gate bypass not working")
	}
	if !foundCommand {
		t.Error("system prompt missing proposed command — LLM can't know what to execute")
	}

	if len(handler.responses) == 0 {
		t.Error("expected a final response from the agent")
	}
}

// TestRun_YesTriggersPendingExecutionNotClassification은 pending action이 활성화된 상태에서
// 사용자가 "예"를 입력하면 분류 없이 직접 executePendingAction으로 라우팅됨을 검증한다.
// 이는 Oracle 버그의 핵심 수정 — classification+intel이 다른 문제(vault.centos.org 등)로
// 점프하는 것을 방지한다.
func TestRun_YesTriggersPendingExecutionNotClassification(t *testing.T) {
	execClient := &sequenceLLMClient{
		responses: []llm.Response{
			// resolveFollowUpDecision (Chat) consumes this first — confirms the pending action
			{Content: `{"kind":"confirm","target":"pending_action","reason":"user said 예"}`},
			// executePendingAction (ChatStream) consumes this
			{Content: "명령 실행 완료."},
		},
	}

	reg := tools.NewRegistry()
	_ = reg.Register(&tools.ProposeActionTool{})

	handler := &captureResponseHandler{}
	a := New(execClient, reg, executor.NewManager(noopAgentExecutor{}), handler, nil)

	pa := &PendingAction{
		ProposedCommand: "sudo -u oracle unzip /home/oracle/LINUX.X64_193000_db_home.zip -d /app/oracle/product/19c/dbhome_1",
		Source:          "tool",
		CreatedAt:       time.Now(),
	}
	a.pendingAction.Activate(pa, 1)
	a.historyTurnCounter = 1

	err := a.Run(context.Background(), "예")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Pending must be consumed (not lingering)
	if a.pendingAction.Current() != nil {
		t.Error("pending action should be consumed after '예'")
	}

	// CONFIRMED ACTION hint must appear in LLM calls
	foundHint := false
	for _, msgs := range execClient.calls {
		for _, msg := range msgs {
			if msg.Role == llm.RoleSystem && strings.Contains(msg.Content, "[CONFIRMED ACTION]") {
				foundHint = true
			}
		}
	}
	if !foundHint {
		t.Error("expected [CONFIRMED ACTION] in LLM system prompt — classification pipeline was NOT bypassed")
	}
}

// TestRun_StalePendingFallsThrough는 만료된 pending action이 자동 소멸되고
// 사용자 입력이 일반 경로로 처리됨을 검증한다.
func TestRun_StalePendingFallsThrough(t *testing.T) {
	// "예"는 shortChatPattern 매칭 → NeedsTools=false → minimal LLM call
	normalClient := &sequenceLLMClient{
		responses: []llm.Response{
			{Content: "알겠습니다."},
		},
	}

	reg := tools.NewRegistry()
	handler := &captureResponseHandler{}
	a := New(normalClient, reg, executor.NewManager(noopAgentExecutor{}), handler, nil)

	stalePA := &PendingAction{
		ProposedCommand: "rm -rf /",
		Source:          "tool",
		CreatedAt:       time.Now().Add(-10 * time.Minute), // 5분 초과 → stale
	}
	a.pendingAction.Activate(stalePA, 0)
	a.historyTurnCounter = 0

	err := a.Run(context.Background(), "예")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Stale pending must be cleared
	if a.pendingAction.Current() != nil {
		t.Error("stale pending action should be cleared")
	}

	// Since stale, the CONFIRMED ACTION path should NOT have been taken
	// (no [CONFIRMED ACTION] in any system prompt)
	foundHint := false
	for _, msgs := range normalClient.calls {
		for _, msg := range msgs {
			if msg.Role == llm.RoleSystem && strings.Contains(msg.Content, "[CONFIRMED ACTION]") {
				foundHint = true
			}
		}
	}
	if foundHint {
		t.Error("stale pending should NOT trigger CONFIRMED ACTION path")
	}
}
