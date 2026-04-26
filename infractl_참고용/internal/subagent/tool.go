// Package subagent
// File: tool.go
// Description: 서브에이전트 분석 LLM 도구 구현
// Responsibility: LLM이 서브에이전트를 병렬 실행하여 종합 분석을 수행할 수 있는 도구 제공

package subagent

import (
	"github.com/yourorg/infractl/internal/tools"
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

// AnalyzeTool은 서브에이전트 병렬 분석을 실행하는 LLM 도구이다.
// EventCb가 설정된 경우 실시간 진행 이벤트를 TUI로 스트리밍한다.
type AnalyzeTool struct {
	Orchestrator *Orchestrator
	EventCb      EventCallback // nil 허용
}

func (t *AnalyzeTool) Name() string { return "subagent_analyze" }
func (t *AnalyzeTool) Description() string {
	return "DB/OS/WAS/보안 전문 서브에이전트를 병렬 실행하여 서버를 종합 분석한다."
}
func (t *AnalyzeTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"server": map[string]interface{}{
				"type":        "string",
				"description": "분석할 서버명 (등록된 서버 이름 또는 빈 문자열이면 로컬)",
			},
			"question": map[string]interface{}{
				"type":        "string",
				"description": "각 서브에이전트에 전달할 분석 질문",
			},
			"agents": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "실행할 에이전트 타입 목록 (db|os|was|security). 비어 있으면 전체 실행",
			},
		},
		"required": []string{"question"},
	}
}
func (t *AnalyzeTool) IsReadOnly() bool            { return true }
func (t *AnalyzeTool) IsEnabled() bool             { return true }

func (t *AnalyzeTool) Execute(ctx context.Context, args map[string]interface{}, _ executor.Executor) (tools.ToolOutcome, error) {
	server, _ := args["server"].(string)
	question, _ := args["question"].(string)

	if question == "" {
		return tools.ToolOutcome{}, fmt.Errorf("question은 필수입니다")
	}

	agentTypes := parseAgentTypes(args["agents"])

	results := t.Orchestrator.AnalyzeParallelWithEvents(ctx, server, question, agentTypes, t.EventCb)
	return tools.ToolOutcome{Content: FormatResults(results), Success: true}, nil
}

// parseAgentTypes는 args의 agents 필드를 AgentType 슬라이스로 변환한다.
func parseAgentTypes(raw interface{}) []AgentType {
	if raw == nil {
		return nil
	}
	list, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	result := make([]AgentType, 0, len(list))
	for _, item := range list {
		s, ok := item.(string)
		if !ok {
			continue
		}
		switch strings.ToLower(s) {
		case "db":
			result = append(result, AgentTypeDB)
		case "os":
			result = append(result, AgentTypeOS)
		case "was":
			result = append(result, AgentTypeWAS)
		case "security":
			result = append(result, AgentTypeSecurity)
		}
	}
	return result
}
