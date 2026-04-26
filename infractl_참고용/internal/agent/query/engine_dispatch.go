// Package query
// File: engine_dispatch.go
// Description: Turn execution and loop dispatch for the query engine.
package query

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/agent/compact"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/tools"
)

func (e *Engine) runLoop(ctx context.Context, p Params, out chan<- QueryEvent) {
	if p.JSONSchema != nil {
		found := false
		for _, td := range p.Tools {
			if td.Function.Name == tools.StructuredOutputToolName {
				found = true
				break
			}
		}
		if !found {
			p.Tools = append(p.Tools, llm.ToolDef{
				Type: "function",
				Function: llm.FunctionDef{
					Name:        tools.StructuredOutputToolName,
					Description: "Use this tool to submit your final response in a structured JSON format matching the schema.",
					Parameters:  p.JSONSchema,
				},
			})
		}
	}

	s := initialState(p.Messages)
	se := NewStreamingExecutor(0)

	for turn := 0; turn < p.MaxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			sendDirect(out, EventTerminal{Reason: TerminalInterrupted})
			return
		}

		slog.Debug("query engine turn start",
			"turn", s.turnCount,
			"tier", p.Tier,
			"model", p.ModelName,
			"messages", len(s.messages),
		)

		if e.compact != nil {
			cs := &compact.State{
				Messages:                     s.messages,
				ContextWindow:                p.ContextWindow,
				SystemTokens:                 p.SystemTokens,
				HasAttemptedReactiveCompact:  s.hasAttemptedReactiveCompact,
				MaxOutputTokensRecoveryCount: s.maxOutputTokensRecoveryCount,
				MaxOutputTokensOverride:      s.maxOutputTokensOverride,
			}
			e.compact.Apply(ctx, cs)
			s.messages = cs.Messages
			s.hasAttemptedReactiveCompact = cs.HasAttemptedReactiveCompact
			s.maxOutputTokensRecoveryCount = cs.MaxOutputTokensRecoveryCount
			s.maxOutputTokensOverride = cs.MaxOutputTokensOverride
		}

		done, nextState := e.runTurn(ctx, p, s, se, out)
		if done {
			return
		}
		s = nextState
	}

	slog.Warn("query engine max turns reached", "max_turns", p.MaxTurns)
	sendDirect(out, EventTerminal{Reason: TerminalMaxTurns, StructuredOutput: s.structuredOutput})
}

func (e *Engine) runTurn(
	ctx context.Context,
	p Params,
	s state,
	se *StreamingExecutor,
	out chan<- QueryEvent,
) (done bool, next state) {
	if !send(ctx, out, EventStreamStart{Tier: string(p.Tier), Model: p.ModelName}) {
		sendDirect(out, EventTerminal{Reason: TerminalInterrupted, StructuredOutput: s.structuredOutput})
		return true, s
	}

	resp, err := e.callLLM(ctx, p, s, out)
	if err != nil {
		if ctx.Err() != nil {
			sendDirect(out, EventTerminal{Reason: TerminalInterrupted, StructuredOutput: s.structuredOutput})
			return true, s
		}

		if e.recovery != nil {
			cs := &compact.State{
				Messages:                     s.messages,
				ContextWindow:                p.ContextWindow,
				SystemTokens:                 p.SystemTokens,
				HasAttemptedReactiveCompact:  s.hasAttemptedReactiveCompact,
				MaxOutputTokensRecoveryCount: s.maxOutputTokensRecoveryCount,
				MaxOutputTokensOverride:      s.maxOutputTokensOverride,
			}
			action, recErr := e.recovery.Handle(ctx, cs, err)
			if recErr != nil {
				slog.Warn("query engine recovery failed", "err", recErr)
			}
			s.messages = cs.Messages
			s.hasAttemptedReactiveCompact = cs.HasAttemptedReactiveCompact
			s.maxOutputTokensRecoveryCount = cs.MaxOutputTokensRecoveryCount
			s.maxOutputTokensOverride = cs.MaxOutputTokensOverride

			switch action {
			case compact.ActionDrainCollapse:
				return false, s.withTransition(ContinueCollapseDrainRetry)
			case compact.ActionReactiveCompact:
				return false, s.withTransition(ContinueReactiveCompactRetry)
			case compact.ActionRetryMaxTokens:
				return false, s.withTransition(ContinueMaxOutputTokensEscalate)
			case compact.ActionStripMedia:
				return false, s.withTransition(ContinueReactiveCompactRetry)
			case compact.ActionStripSignaturesAndRetry:
				return false, s.withTransition(ContinueReactiveCompactRetry)
			case compact.ActionMultiTurnRecovery:
				return false, s.withTransition(ContinueMaxOutputTokensRecovery)
			case compact.ActionSurfaceError:
				// fallthrough to terminal model error
			}
		}

		send(ctx, out, EventError{Err: err, Recoverable: false})
		sendDirect(out, EventTerminal{Reason: TerminalModelError, Err: err, StructuredOutput: s.structuredOutput})
		return true, s
	}

	autoToolCalls := drainAutoToolCalls(p.DequeueAutoTool, 3)
	if len(autoToolCalls) > 0 {
		resp.ToolCalls = append(resp.ToolCalls, autoToolCalls...)
	}
	send(ctx, out, EventAssistantResponse{Text: resp.Content, ToolCalls: resp.ToolCalls})

	if len(resp.ToolCalls) == 0 && strings.TrimSpace(resp.Content) == "" {
		if s.emptyResponseRetries >= 1 {
			err := fmt.Errorf("model returned empty response repeatedly")
			slog.Error("empty response repeated; aborting turn", "turn", s.turnCount, "retries", s.emptyResponseRetries+1)
			send(ctx, out, EventError{Err: err, Recoverable: false})
			sendDirect(out, EventTerminal{Reason: TerminalModelError, Err: err, StructuredOutput: s.structuredOutput})
			return true, s
		}

		slog.Warn("empty response from model, requesting retry")
		retryMsg := llm.Message{
			Role:    llm.RoleUser,
			Content: "Error: Your previous response was empty. If you are still working, please continue. If you have finished, please provide a final summary.",
		}
		updated := make([]llm.Message, 0, len(s.messages)+2)
		updated = append(updated, s.messages...)
		updated = append(updated, llm.Message{Role: llm.RoleAssistant, Content: "(empty response)"})
		updated = append(updated, retryMsg)

		next = s
		next.messages = updated
		next.turnCount = s.turnCount + 1
		next.emptyResponseRetries = s.emptyResponseRetries + 1
		next = next.withTransition(ContinueNextTurn)
		return false, next
	}
	s.emptyResponseRetries = 0

	if p.JSONSchema != nil && len(resp.ToolCalls) == 0 {
		retryMsg := llm.Message{
			Role:    llm.RoleUser,
			Content: fmt.Sprintf("Error: You must provide your final answer using the '%s' tool with the specified JSON schema. Please retry.", tools.StructuredOutputToolName),
		}
		updated := make([]llm.Message, 0, len(s.messages)+2)
		updated = append(updated, s.messages...)
		updated = append(updated, llm.Message{Role: llm.RoleAssistant, Content: resp.Content})
		updated = append(updated, retryMsg)

		next = s
		next.messages = updated
		next.turnCount = s.turnCount + 1
		next = next.withTransition(ContinueNextTurn)
		return false, next
	}

	if len(resp.ToolCalls) == 0 {
		if shouldResearchAutoContinue(s, resp) {
			retryMsg := llm.Message{
				Role: llm.RoleUser,
				Content: "The previous research reported insufficient results (no sources / budget exhausted / follow-up required). " +
					"Please continue investigating: reformulate queries with different angles, or fetch the most promising URLs with web_fetch. " +
					"Do not answer from snippets alone - gather authoritative sources first.",
			}
			updated := make([]llm.Message, 0, len(s.messages)+2)
			updated = append(updated, s.messages...)
			updated = append(updated, llm.Message{Role: llm.RoleAssistant, Content: resp.Content})
			updated = append(updated, retryMsg)

			next = s
			next.messages = updated
			next.turnCount = s.turnCount + 1
			next.researchAutoContinueCount = s.researchAutoContinueCount + 1
			next = next.withTransition(ContinueResearchAuto)
			slog.Warn("research auto-continue triggered", "count", next.researchAutoContinueCount)
			return false, next
		}

		sendDirect(out, EventTerminal{Reason: TerminalCompleted, StructuredOutput: s.structuredOutput})
		return true, s
	}

	if isIdenticalToolCalls(resp.ToolCalls, s.lastToolCalls) {
		slog.Warn("tight-loop detected: consecutive identical tool calls", "tool_calls", len(resp.ToolCalls))
		retryMsg := llm.Message{
			Role:    llm.RoleUser,
			Content: "Error: You are repeatedly calling the same tool with the same arguments. If the previous output was not what you expected, please try a different approach or different arguments.",
		}
		updated := make([]llm.Message, 0, len(s.messages)+2)
		updated = append(updated, s.messages...)
		updated = append(updated, llm.Message{Role: llm.RoleAssistant, Content: resp.Content, ToolCalls: resp.ToolCalls})
		updated = append(updated, retryMsg)

		next = s
		next.messages = updated
		next.turnCount = s.turnCount + 1
		next.lastToolCalls = resp.ToolCalls
		next = next.withTransition(ContinueNextTurn)
		return false, next
	}

	batches := PartitionToolCalls(resp.ToolCalls, p.Registry)
	toolResultMsgs, toolInfos, aborted := se.Execute(ctx, batches, out, p.RunTool)
	if aborted {
		sendDirect(out, EventTerminal{Reason: TerminalAbortedTools, StructuredOutput: s.structuredOutput})
		return true, s
	}

	next = s
	next.lastToolCalls = resp.ToolCalls
	next.lastResearchMetadata = ""
	for _, info := range toolInfos {
		if info.Name == "web_search" || info.Name == "web_fetch" {
			next.lastResearchMetadata = info.MetadataJSON
			continue
		}
		next.researchAutoContinueCount = 0
	}

	hasStructuredOutput := false
	for _, tc := range resp.ToolCalls {
		if tc.Function.Name == tools.StructuredOutputToolName {
			args := ParseArgsForCallback(tc.Function.Arguments)
			next.structuredOutput = args
			hasStructuredOutput = true
			break
		}
	}

	assistantMsg := llm.Message{
		Role:      llm.RoleAssistant,
		ToolCalls: resp.ToolCalls,
	}
	updated := make([]llm.Message, 0, len(s.messages)+1+len(toolResultMsgs))
	updated = append(updated, s.messages...)
	updated = append(updated, assistantMsg)
	updated = append(updated, toolResultMsgs...)

	next.messages = updated
	next.turnCount = s.turnCount + 1
	next = next.withTransition(ContinueNextTurn)

	if hasStructuredOutput {
		sendDirect(out, EventTerminal{Reason: TerminalCompleted, StructuredOutput: next.structuredOutput})
		return true, next
	}

	slog.Debug("query engine turn complete",
		"turn", s.turnCount,
		"tool_calls", len(resp.ToolCalls),
		"input_tokens", resp.InputTokens,
		"output_tokens", resp.OutputTokens,
	)

	return false, next
}

func (e *Engine) callLLM(
	ctx context.Context,
	p Params,
	s state,
	out chan<- QueryEvent,
) (llm.Response, error) {
	onThinking := func(tok string) {
		send(ctx, out, EventAssistantChunk{Text: tok, Thinking: true})
	}
	onToken := func(tok string) {
		send(ctx, out, EventAssistantChunk{Text: tok, Thinking: false})
	}

	msgs := make([]llm.Message, 0, 1+len(s.messages))
	msgs = append(msgs, p.SystemMsg)
	msgs = append(msgs, s.messages...)

	var opts []llm.CallOption
	if s.maxOutputTokensOverride != nil {
		opts = append(opts, llm.WithMaxTokens(*s.maxOutputTokensOverride))
	}

	retryCfg := llm.DefaultRetryConfig()
	var lastErr error
	for attempt := 0; attempt <= retryCfg.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := llm.RetryDelay(attempt, retryCfg, lastErr)
			slog.Warn("llm transient error, retrying with backoff",
				"attempt", attempt, "max_retries", retryCfg.MaxRetries, "delay", delay, "err", lastErr)
			select {
			case <-ctx.Done():
				return llm.Response{}, fmt.Errorf("llm chat stream (turn %d): cancelled during retry: %w", s.turnCount, ctx.Err())
			case <-time.After(delay):
			}
		}

		resp, err := p.Client.ChatStream(ctx, msgs, p.Tools, nil, onThinking, onToken, opts...)
		if err == nil {
			return resp, nil
		}
		if !llm.IsTransient(err) {
			return llm.Response{}, fmt.Errorf("llm chat stream (turn %d): %w", s.turnCount, err)
		}
		lastErr = err
	}
	return llm.Response{}, fmt.Errorf("llm chat stream (turn %d): %w", s.turnCount, lastErr)
}

func isIdenticalToolCalls(a, b []llm.ToolCall) bool {
	if len(a) != len(b) || len(a) == 0 {
		return false
	}
	for i := range a {
		if a[i].Function.Name != b[i].Function.Name || a[i].Function.Arguments != b[i].Function.Arguments {
			return false
		}
	}
	return true
}

type researchMetadata struct {
	FollowUpRequired bool   `json:"follow_up_required"`
	CompletionReason string `json:"completion_reason"`
}

func parseResearchMetadata(metadataJSON string) researchMetadata {
	if strings.TrimSpace(metadataJSON) == "" {
		return researchMetadata{}
	}
	var meta researchMetadata
	_ = json.Unmarshal([]byte(metadataJSON), &meta)
	return meta
}

func shouldResearchAutoContinue(s state, resp llm.Response) bool {
	if len(resp.ToolCalls) != 0 {
		return false
	}
	if s.researchAutoContinueCount >= 2 {
		return false
	}
	if s.lastResearchMetadata == "" {
		return false
	}
	meta := parseResearchMetadata(s.lastResearchMetadata)
	if meta.FollowUpRequired {
		return true
	}
	switch meta.CompletionReason {
	case "no_results", "budget_exhausted_no_sources", "search_error":
		return true
	default:
		return false
	}
}

func drainAutoToolCalls(dequeue func() (llm.ToolCall, bool), max int) []llm.ToolCall {
	if dequeue == nil || max <= 0 {
		return nil
	}
	out := make([]llm.ToolCall, 0, max)
	for len(out) < max {
		tc, ok := dequeue()
		if !ok {
			break
		}
		out = append(out, tc)
	}
	return out
}
