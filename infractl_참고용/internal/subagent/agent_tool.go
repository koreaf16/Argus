// Package subagent
// File: agent_tool.go
// Description: 범용 서브에이전트 위임 도구 구현
// Responsibility: 임의의 작업을 서브에이전트에 위임하는 delegate_task 도구 제공

package subagent

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/tools"
)

const agentTypeDelegated AgentType = "delegate"

// DelegateAgentTool은 임의의 작업을 서브에이전트에 위임하는 LLM 도구이다.
// 메인 에이전트 루프와 독립적으로 실행되어 병렬 작업 위임이 가능하다.
type DelegateAgentTool struct {
	Runner  *Runner
	EventCb EventCallback // nil 허용
}

func (t *DelegateAgentTool) Name() string        { return "delegate_task" }
func (t *DelegateAgentTool) IsReadOnly() bool     { return true }
func (t *DelegateAgentTool) IsEnabled() bool      { return true }

func (t *DelegateAgentTool) Description() string {
	return "Delegate a task to a sub-agent for independent execution. The sub-agent has access to all read-only tools and uses the fast-tier LLM. Use for parallel data gathering or analysis tasks."
}

func (t *DelegateAgentTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "Task description for the sub-agent. Be specific about what information to gather.",
			},
			"server": map[string]interface{}{
				"type":        "string",
				"description": "Target server name (registered server or empty for local). The sub-agent will use this as the default execution target.",
			},
			"allowed_tools": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Optional: restrict sub-agent to specific tool names or prefixes (e.g. [\"shell_exec\", \"oracle_\"]). If empty, all read-only tools are allowed.",
			},
		},
		"required": []string{"prompt"},
	}
}

func (t *DelegateAgentTool) Execute(ctx context.Context, args map[string]interface{}, _ executor.Executor) (tools.ToolOutcome, error) {
	prompt, ok := args["prompt"].(string)
	if !ok || strings.TrimSpace(prompt) == "" {
		return tools.ToolOutcome{}, fmt.Errorf("prompt is required")
	}
	server, _ := args["server"].(string)
	allowedTools := parseStringSlice(args["allowed_tools"])

	localRunner := *t.Runner
	localRunner.eventCb = t.EventCb
	if len(allowedTools) > 0 {
		// filter를 overriding하여 지정한 도구만 허용
		localRunner.registry = filteredRegistry(t.Runner.registry, allowedTools)
	}
	// read-only 도구만 허용 (mutation 도구 차단)
	localRunner.registry = readOnlyRegistry(localRunner.registry)

	cfg := SubagentConfig{
		Type:     agentTypeDelegated,
		Server:   server,
		Question: prompt,
	}

	localRunner.fire(Event{Type: EventAgentStart, AgentType: agentTypeDelegated, Server: server})

	result := localRunner.executeDelegate(ctx, cfg)
	if result.Err != nil {
		return tools.ToolOutcome{}, fmt.Errorf("delegate_task: %w", result.Err)
	}
	return tools.ToolOutcome{Content: result.Answer, Success: true}, nil
}

// executeDelegate는 delegate_task용 간소화된 실행 루프이다.
// filterTools를 우회하고 registry를 직접 사용한다.
func (r *Runner) executeDelegate(ctx context.Context, cfg SubagentConfig) SubagentResult {
	result := SubagentResult{Type: cfg.Type, Server: cfg.Server}

	allTools := r.registry.GetEnabled()
	// read-only 필터는 이미 registry에 적용되어 있음
	toolDefs := toToolDefs(allTools)

	systemMsg := llm.Message{Role: llm.RoleSystem, Content: delegateSystemPrompt(cfg.Server)}
	userMsg := llm.Message{Role: llm.RoleUser, Content: cfg.Question}
	messages := []llm.Message{systemMsg, userMsg}

	client := r.resolveClient()
	for i := 0; i < maxSubagentIter; i++ {
		resp, err := client.Chat(ctx, messages, toolDefs, nil)
		if err != nil {
			result.Err = fmt.Errorf("delegate llm call: %w", err)
			return result
		}
		result.InputTokens += resp.InputTokens
		result.OutputTokens += resp.OutputTokens

		// cost tracking은 상위 Runner.Execute()에서 이미 처리됨 — 여기선 생략

		if len(resp.ToolCalls) == 0 {
			result.Answer = resp.Content
			return result
		}

		assistantMsg := llm.Message{Role: llm.RoleAssistant, ToolCalls: resp.ToolCalls}
		messages = append(messages, assistantMsg)
		for _, tc := range resp.ToolCalls {
			r.fire(Event{Type: EventToolCall, AgentType: cfg.Type, Server: cfg.Server, ToolName: tc.Function.Name})
			toolResult := r.executeTool(ctx, tc, cfg.Server, allTools)
			llm.LogToolResult(tc.Function.Name, toolResult.Content)
			result.ToolUses++
			messages = append(messages, toolResult)
		}
	}
	result.Err = fmt.Errorf("delegate_task 최대 반복 횟수(%d) 초과", maxSubagentIter)
	return result
}

// filteredRegistry는 원본 registry에서 허용 도구만 포함하는 래퍼를 반환한다.
func filteredRegistry(reg *tools.Registry, allowedPrefixes []string) *tools.Registry {
	newReg := tools.NewRegistry()
	for _, t := range reg.GetEnabled() {
		for _, prefix := range allowedPrefixes {
			if t.Name() == prefix || strings.HasPrefix(t.Name(), prefix) {
				_ = newReg.Register(t)
				break
			}
		}
	}
	return newReg
}

// readOnlyRegistry는 원본 registry에서 read-only 도구만 포함하는 래퍼를 반환한다.
func readOnlyRegistry(reg *tools.Registry) *tools.Registry {
	newReg := tools.NewRegistry()
	for _, t := range reg.GetEnabled() {
		if t.IsReadOnly() {
			_ = newReg.Register(t)
		}
	}
	return newReg
}

// parseStringSlice는 args에서 []string을 추출한다.
func parseStringSlice(raw interface{}) []string {
	if raw == nil {
		return nil
	}
	list, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok && s != "" {
			result = append(result, s)
		}
	}
	return result
}

// delegateSystemPrompt는 delegate_task 서브에이전트용 시스템 프롬프트를 반환한다.
func delegateSystemPrompt(server string) string {
	target := "로컬 환경"
	if server != "" && server != "localhost" {
		target = server + " 서버"
	}
	return fmt.Sprintf(`당신은 인프라 작업 위임 에이전트입니다.
대상: %s
제공된 read-only 도구를 사용하여 요청된 작업을 수행하고 결과를 반환하세요.
데이터를 직접 수집하고 구체적인 수치와 상태를 포함한 명확한 결과를 제공하세요.`, target)
}
