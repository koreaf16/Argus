// Package subagent
// File: orchestrator.go
// Description: 복수 서브에이전트 병렬 실행 오케스트레이터
// Responsibility: AnalyzeParallel / AnalyzeParallelWithEvents API를 RunParallel로 위임, 결과 포맷팅

package subagent

import (
	"context"
	"fmt"
	"strings"
)

// Orchestrator는 복수 서브에이전트를 병렬로 실행한다.
type Orchestrator struct {
	runner      *Runner
	maxParallel int // Phase F: errgroup 동시성 제한 (0 = 무제한)
}

// NewOrchestrator는 Orchestrator를 생성한다.
func NewOrchestrator(runner *Runner) *Orchestrator {
	return &Orchestrator{runner: runner}
}

// SetMaxParallel은 동시 실행 상한을 설정한다 (Phase F: config.Subagent.MaxParallel 주입).
func (o *Orchestrator) SetMaxParallel(n int) {
	o.maxParallel = n
}

// Runner는 내부 Runner를 반환한다 (DelegateAgentTool 등에서 사용).
func (o *Orchestrator) Runner() *Runner {
	return o.runner
}

// AnalyzeParallel은 지정된 AgentType 목록을 병렬로 실행하고 결과를 반환한다.
// server가 빈 문자열이면 로컬 실행으로 폴백된다.
func (o *Orchestrator) AnalyzeParallel(ctx context.Context, server, question string, agentTypes []AgentType) []SubagentResult {
	return o.AnalyzeParallelWithEvents(ctx, server, question, agentTypes, nil)
}

// AnalyzeParallelWithEvents는 EventCallback을 받아 실시간 진행 이벤트를 발사하면서 병렬 실행한다.
// cb가 nil이면 AnalyzeParallel과 동일하게 동작한다.
//
// Phase F: sync.WaitGroup → RunParallel(errgroup) 으로 교체.
// - 첫 에러 발생 시 공유 ctx 취소 → sibling이 다음 Chat 호출에서 ctx.Done() 감지 후 종료.
// - maxParallel > 0이면 errgroup.SetLimit으로 동시성을 제한한다.
func (o *Orchestrator) AnalyzeParallelWithEvents(ctx context.Context, server, question string, agentTypes []AgentType, cb EventCallback) []SubagentResult {
	if len(agentTypes) == 0 {
		agentTypes = AllAgentTypes
	}

	configs := make([]SubagentConfig, len(agentTypes))
	for i, t := range agentTypes {
		configs[i] = SubagentConfig{Type: t, Server: server, Question: question}
	}

	opts := RunOpts{
		MaxParallel:     o.maxParallel,
		ContinueOnError: false, // 기본: sibling cancel 활성
		EventCb:         cb,
	}

	return RunParallel(ctx, configs, opts, o.runner)
}

// FormatResults는 서브에이전트 결과를 사람이 읽기 쉬운 문자열로 변환한다.
func FormatResults(results []SubagentResult) string {
	if len(results) == 0 {
		return "분석 결과 없음."
	}

	var sb strings.Builder
	sb.WriteString("=== 서브에이전트 종합 분석 결과 ===\n\n")

	for _, r := range results {
		label := agentTypeLabel(r.Type)
		sb.WriteString(fmt.Sprintf("▶ [%s 에이전트] 서버: %s\n", label, r.Server))

		if r.Err != nil {
			sb.WriteString(fmt.Sprintf("  오류: %s\n\n", r.Err))
			continue
		}

		answer := strings.TrimSpace(r.Answer)
		if answer == "" {
			answer = "(분석 결과 없음)"
		}
		sb.WriteString(answer)
		sb.WriteString(fmt.Sprintf("\n  [토큰: 입력 %d / 출력 %d]\n\n", r.InputTokens, r.OutputTokens))
	}

	return sb.String()
}

func agentTypeLabel(t AgentType) string {
	switch t {
	case AgentTypeDB:
		return "DB"
	case AgentTypeOS:
		return "OS"
	case AgentTypeWAS:
		return "WAS"
	case AgentTypeSecurity:
		return "보안"
	default:
		return string(t)
	}
}
