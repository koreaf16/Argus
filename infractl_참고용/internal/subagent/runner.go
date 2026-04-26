// Package subagent
// File: runner.go
// Description: 단일 서브에이전트 미니 루프 실행기
// Responsibility: 제한된 도구 목록으로 LLM Chat + 도구 실행을 최대 N회 반복하여 결과 반환

package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/cost"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/tools"
)

const maxSubagentIter = 10

// Runner는 단일 서브에이전트 실행 루프를 담당한다.
type Runner struct {
	llmClient   llm.Client
	llmRegistry *llm.Registry // nil 허용 — 있으면 fast 티어 우선 사용
	registry    *tools.Registry
	execMgr     *executor.Manager
	costTracker *cost.Tracker  // nil 허용 — 없으면 비용 기록 생략
	eventCb     EventCallback  // nil 허용 — 설정 시 실시간 진행 이벤트 발사
}

// NewRunner는 Runner를 생성한다.
func NewRunner(lc llm.Client, reg *tools.Registry, mgr *executor.Manager, ct *cost.Tracker) *Runner {
	return &Runner{llmClient: lc, registry: reg, execMgr: mgr, costTracker: ct}
}

// SetCostTracker는 비용 추적기를 주입한다 (생성 후 주입 가능).
func (r *Runner) SetCostTracker(ct *cost.Tracker) {
	r.costTracker = ct
}

// SetEventCallback은 실시간 진행 이벤트 콜백을 주입한다.
func (r *Runner) SetEventCallback(cb EventCallback) {
	r.eventCb = cb
}

// SetLLMRegistry는 멀티 LLM 레지스트리를 주입한다.
// 주입하면 서브에이전트 실행 시 fast 티어를 우선 사용하고, 없으면 general로 폴백한다.
func (r *Runner) SetLLMRegistry(reg *llm.Registry) {
	r.llmRegistry = reg
}

// resolveClient는 서브에이전트용 LLM 클라이언트를 결정한다.
// llmRegistry가 있으면 fast 티어를 우선 시도하고, 없으면 llmClient를 사용한다.
func (r *Runner) resolveClient() llm.Client {
	if r.llmRegistry != nil {
		if c, _, err := r.llmRegistry.Resolve(llm.TierFast); err == nil {
			return c
		}
	}
	return r.llmClient
}

// fire는 eventCb가 설정된 경우 이벤트를 발사한다.
func (r *Runner) fire(e Event) {
	if r.eventCb != nil {
		r.eventCb(e)
	}
}

// Execute는 SubagentConfig를 실행하고 결과를 반환한다.
func (r *Runner) Execute(ctx context.Context, cfg SubagentConfig) SubagentResult {
	result := SubagentResult{Type: cfg.Type, Server: cfg.Server}
	start := time.Now()

	r.fire(Event{Type: EventAgentStart, AgentType: cfg.Type, Server: cfg.Server})

	filteredTools := r.filterTools(cfg.Type)
	toolDefs := toToolDefs(filteredTools)

	systemMsg := llm.Message{Role: llm.RoleSystem, Content: SystemPrompt(cfg.Type, cfg.Server)}
	userMsg := llm.Message{Role: llm.RoleUser, Content: cfg.Question}

	client := r.resolveClient()

	// Qwen 인라인 모드 대응: 클라이언트가 인라인 모드이면 시스템 프롬프트에 도구 인덱스 주입
	if ic, ok := client.(llm.InlineToolCallModeClient); ok && ic.IsInlineToolCalls() {
		var sb strings.Builder
		sb.WriteString(systemMsg.Content)
		sb.WriteString("\n\n")
		tools.AppendQwenToolGuideline(&sb)
		tools.AppendQwenToolIndex(&sb, filteredTools)
		systemMsg.Content = sb.String()
	}

	messages := []llm.Message{systemMsg, userMsg}

	for i := 0; i < maxSubagentIter; i++ {
		resp, err := client.Chat(ctx, messages, toolDefs, nil)
		if err != nil {
			result.Err = fmt.Errorf("subagent llm call: %w", err)
			r.fire(Event{
				Type: EventAgentDone, AgentType: cfg.Type, Server: cfg.Server,
				ToolUses: result.ToolUses, InputTokens: result.InputTokens,
				OutputTokens: result.OutputTokens, Duration: time.Since(start), Success: false,
			})
			return result
		}

		result.InputTokens += resp.InputTokens
		result.OutputTokens += resp.OutputTokens

		if r.costTracker != nil && (resp.InputTokens > 0 || resp.OutputTokens > 0) {
			r.costTracker.Record(ctx, "", resp.InputTokens, resp.OutputTokens, cost.SourceSubagent, 0)
		}

		if len(resp.ToolCalls) == 0 {
			// If no tool calls were made on the first iteration the model likely generated
			// planning text or hallucinated results instead of executing tools.
			// Nudge once to force actual tool execution before accepting the answer.
			if i == 0 && result.ToolUses == 0 && cfg.Type == AgentTypeIntel {
				slog.Warn("intel subagent returned text without tool calls on first iteration — nudging",
					"content_len", len(resp.Content))
				messages = append(messages, llm.Message{Role: llm.RoleAssistant, Content: resp.Content})
				messages = append(messages, llm.Message{
					Role: llm.RoleUser,
					Content: "도구를 실제로 호출해야 합니다. 계획 텍스트나 가상 결과가 아닌, " +
						"실제 system_info/disk_usage/shell_exec/web_search 도구를 지금 즉시 호출하세요.",
				})
				continue
			}
			// Intel subagent가 단 한 번도 실제 도구를 실행하지 않은 경우 —
			// LLM이 JSON 계획 텍스트를 출력만 하고 도구를 호출하지 않은 것이므로
			// 할루시네이션 출력을 폐기하여 상위 에이전트의 시스템 프롬프트를 오염시키지 않는다.
			if cfg.Type == AgentTypeIntel && result.ToolUses == 0 {
				slog.Warn("intel subagent: no tools executed — discarding hallucinated plan output",
					"content_len", len(resp.Content))
				result.Answer = ""
				r.fire(Event{
					Type: EventAgentDone, AgentType: cfg.Type, Server: cfg.Server,
					ToolUses: 0, InputTokens: result.InputTokens,
					OutputTokens: result.OutputTokens, Duration: time.Since(start), Success: false,
				})
				return result
			}
			result.Answer = resp.Content
			r.fire(Event{
				Type: EventAgentDone, AgentType: cfg.Type, Server: cfg.Server,
				ToolUses: result.ToolUses, InputTokens: result.InputTokens,
				OutputTokens: result.OutputTokens, Duration: time.Since(start), Success: true,
			})
			return result
		}

		assistantMsg := llm.Message{Role: llm.RoleAssistant, ToolCalls: resp.ToolCalls}
		messages = append(messages, assistantMsg)

		for _, tc := range resp.ToolCalls {
			r.fire(Event{
				Type: EventToolCall, AgentType: cfg.Type, Server: cfg.Server,
				ToolName: tc.Function.Name,
				ToolArg:  tc.Function.Arguments,
			})
			toolResult := r.executeTool(ctx, tc, cfg.Server, filteredTools)
			// 로그에는 원본 전체 내용을 기록
			llm.LogToolResult(tc.Function.Name, toolResult.Content)

			// 서브에이전트 컨텍스트 보호를 위해 길이 제한 (타이트하게 2000자)
			const maxSubContextLen = 2000
			if len(toolResult.Content) > maxSubContextLen {
				toolResult.Content = toolResult.Content[:maxSubContextLen] +
					"\n\n[... content truncated due to length ...]"
				slog.Debug("truncated subagent tool result",
					"agent", cfg.Type, "tool", tc.Function.Name, "original_len", len(toolResult.Content))
			}

			result.ToolUses++
			messages = append(messages, toolResult)
		}
	}

	result.Err = fmt.Errorf("서브에이전트 최대 반복 횟수(%d) 초과", maxSubagentIter)
	r.fire(Event{
		Type: EventAgentDone, AgentType: cfg.Type, Server: cfg.Server,
		ToolUses: result.ToolUses, InputTokens: result.InputTokens,
		OutputTokens: result.OutputTokens, Duration: time.Since(start), Success: false,
	})
	return result
}

// filterTools는 AgentType에 허용된 도구만 필터링하여 반환한다.
func (r *Runner) filterTools(t AgentType) []tools.Tool {
	all := r.registry.GetEnabled()
	result := make([]tools.Tool, 0, len(all))
	for _, tool := range all {
		if ToolAllowed(t, tool.Name()) {
			result = append(result, tool)
		}
	}
	return result
}

// executeTool은 단일 도구 호출을 실행하고 tool 역할 메시지를 반환한다.
func (r *Runner) executeTool(ctx context.Context, tc llm.ToolCall, server string, toolList []tools.Tool) llm.Message {
	var found tools.Tool
	for _, t := range toolList {
		if t.Name() == tc.Function.Name {
			found = t
			break
		}
	}
	if found == nil {
		return llm.Message{
			Role:       llm.RoleTool,
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("Error: tool not found: %s", tc.Function.Name),
		}
	}

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return llm.Message{
			Role:       llm.RoleTool,
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("Error: invalid arguments: %s", err),
		}
	}

	target := tools.ExtractTarget(args)
	if target == "" {
		target = server
	}

	exec, err := r.execMgr.Get(target)
	if err != nil {
		slog.Warn("subagent executor not found", "target", target, "err", err)
		exec, _ = r.execMgr.Get("") // fallback to local
	}

	output, err := found.Execute(ctx, args, exec)
	if err != nil {
		return llm.Message{
			Role:       llm.RoleTool,
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("Error: %s", err),
		}
	}
	return llm.Message{Role: llm.RoleTool, ToolCallID: tc.ID, Content: output.Content}
}

// toToolDefs는 도구 목록을 LLM API용 ToolDef로 변환한다.
func toToolDefs(toolList []tools.Tool) []llm.ToolDef {
	defs := make([]llm.ToolDef, len(toolList))
	for i, t := range toolList {
		defs[i] = llm.ToolDef{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			},
		}
	}
	return defs
}
