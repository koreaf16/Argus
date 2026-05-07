package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	ctxpkg "github.com/koreaf16/argus/internal/context"
	"github.com/koreaf16/argus/internal/hooks"
	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/services/tools"
	"github.com/koreaf16/argus/internal/state"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
	"github.com/koreaf16/argus/internal/utils/permissions"
)

const subQueryMaxTokens = 512

func (e *Engine) ExecuteSubQuery(ctx context.Context, systemPrompt string, userPrompt string) (string, error) {
	e.mu.RLock()
	client := e.llm
	e.mu.RUnlock()

	req := llm.Request{
		Model: "", // Uses client's default model
		System: []llm.SystemBlock{
			{Type: "text", Text: systemPrompt},
		},
		Messages: []llm.Message{
			llm.TextMessage(llm.RoleUser, userPrompt),
		},
		MaxTokens: subQueryMaxTokens,
	}

	stream, err := client.Stream(ctx, req)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	for event := range stream {
		if event.Kind == llm.EventError {
			return "", event.Err
		}
		if event.Kind == llm.EventTextDelta {
			sb.WriteString(event.Delta)
		}
	}
	return sb.String(), nil
}

func (e *Engine) classificationFunctions(ctx context.Context, turnIndex int) (permissions.SubQueryFunc, permissions.SearchFunc, permissions.FetchFunc) {
	subQuery := func(tCtx context.Context, systemPrompt, userPrompt string) (string, error) {
		return e.ExecuteSubQuery(tCtx, systemPrompt, userPrompt)
	}
	search := func(tCtx context.Context, query string) (string, error) {
		searchToolNames := []string{"google_web_search", "web_search", "mcp__google_search__search"}
		var targetName string
		var ok bool
		for _, name := range searchToolNames {
			if _, ok = e.registry.Lookup(name); ok {
				targetName = name
				break
			}
		}
		if !ok {
			return "", fmt.Errorf("search tool not found")
		}
		input, _ := json.Marshal(map[string]any{"query": query})
		out, isErr := e.invokeTool(tCtx, nil, llm.ToolUseStart{Name: targetName, Input: input, ID: "icsa-search"}, e.registry, e.state, e.deps, true, turnIndex)
		if isErr {
			return "", fmt.Errorf("search failed: %s", out)
		}
		return out, nil
	}
	fetch := func(tCtx context.Context, url string) (string, error) {
		fetchToolNames := []string{"web_fetch", "webfetch", "mcp__web_fetch__fetch"}
		var targetName string
		var ok bool
		for _, name := range fetchToolNames {
			if _, ok = e.registry.Lookup(name); ok {
				targetName = name
				break
			}
		}
		if !ok {
			return "", fmt.Errorf("fetch tool not found")
		}
		input, _ := json.Marshal(map[string]any{"url": url})
		out, isErr := e.invokeTool(tCtx, nil, llm.ToolUseStart{Name: targetName, Input: input, ID: "icsa-fetch"}, e.registry, e.state, e.deps, true, turnIndex)
		if isErr {
			return "", fmt.Errorf("fetch failed: %s", out)
		}
		return out, nil
	}
	return subQuery, search, fetch
}

func (e *Engine) ExecutePlannedStep(ctx context.Context, step PlannedStep) (string, error) {
	toolName := strings.ToLower(strings.TrimSpace(step.Tool))
	if toolName != "bash" && toolName != "powershell" {
		return "", fmt.Errorf("unsupported planned step tool: %s", step.Tool)
	}
	prompt := strings.TrimSpace(step.Prompt)
	if prompt == "" {
		return "", fmt.Errorf("planned step prompt is empty")
	}

	e.mu.RLock()
	registry := e.registry
	appState := e.state
	deps := e.deps
	e.mu.RUnlock()

	callInput, err := buildPlannedStepToolCallInput(step, deps.WorkingDir)
	if err != nil {
		return "", err
	}
	call := llm.ToolUseStart{
		ID:    "planned-step",
		Name:  toolName,
		Input: callInput,
	}

	result, isErr := e.invokeTool(ctx, nil, call, registry, appState, deps, true, 0)
	if isErr {
		return "", errors.New(result)
	}
	return result, nil
}

func (e *Engine) SubmitMessage(ctx context.Context, userInput string) (<-chan UIEvent, error) {
	text := strings.TrimSpace(userInput)
	if text == "" {
		return nil, fmt.Errorf("empty user input")
	}

	e.mu.Lock()
	dispatcher := e.hookDispatcher
	preDeps := e.deps
	turnIndex := 0
	if preDeps.AIDebug.Enabled && preDeps.AIDebug.Emitter != nil {
		e.debugTurn++
		turnIndex = e.debugTurn
	}
	e.mu.Unlock()

	if dispatcher != nil && !dispatcher.IsEmpty() {
		t0 := time.Now()
		agg := dispatcher.Dispatch(ctx, types.HookEventUserPrompt, hooks.HookInput{
			UserPrompt: text,
		})
		e.emitTrace(turnIndex, "hook.dispatch", "", "", "", map[string]any{
			"event":        "UserPromptSubmit",
			"continue":     agg.Continue,
			"block_reason": agg.BlockReason,
			"duration_ms":  time.Since(t0).Milliseconds(),
		})
		if !agg.Continue {
			errCh := make(chan UIEvent, 1)
			reason := agg.BlockReason
			if reason == "" {
				reason = "UserPromptSubmit hook blocked the prompt"
			}
			errCh <- UIEvent{Kind: UIEventError, Err: fmt.Errorf("hook: %s", reason)}
			close(errCh)
			return errCh, nil
		}
	}

	e.mu.Lock()
	client := e.llm
	if client == nil {
		e.mu.Unlock()
		return nil, fmt.Errorf("llm is not configured")
	}
	e.messages = append(e.messages, newUserTextMessage(text))
	e.graph.AppendUser(text)
	cfg := e.cfg
	systemFn := e.systemFn
	registry := e.registry
	appState := e.state
	deps := e.deps
	if appState != nil {
	}
	stopHooksCopy := append([]StopHook(nil), e.stopHooks...)
	hookDispatcher := e.hookDispatcher
	currentUserText := text
	if turnIndex == 0 && deps.AIDebug.Enabled && deps.AIDebug.Emitter != nil {
		e.debugTurn++
		turnIndex = e.debugTurn
	}
	e.mu.Unlock()

	if cfg.MaxToolIterations <= 0 {
		cfg.MaxToolIterations = 1
	}
	cfg = applyPersistenceDefaults(cfg)

	out := make(chan UIEvent, 64)
	go e.run(ctx, out, client, cfg, systemFn, registry, appState, deps, stopHooksCopy, hookDispatcher, currentUserText, turnIndex)
	return out, nil
}

func (e *Engine) run(
	ctx context.Context,
	out chan<- UIEvent,
	client llm.LLM,
	cfg Config,
	systemFn func() []llm.SystemBlock,
	registry *tool.Registry,
	appState *state.AppState,
	deps Deps,
	stopHooks []StopHook,
	hookDispatcher *hooks.HookDispatcher,
	currentUserText string,
	turnIndex int,
) {
	defer close(out)
	defer func() {
		if r := recover(); r != nil {
			out <- UIEvent{Kind: UIEventError, Err: fmt.Errorf("engine panic: %v", r)}
		}
	}()

	turnStartedAt := time.Now().UTC()
	turnStop := llm.StopReasonUnknown
	turnError := false
	var lastContextUsage map[string]any
	if turnIndex > 0 {
		e.emitTrace(turnIndex, "turn.start", client.Provider(), "", "", map[string]any{
			"user_input": currentUserText,
		})
	}
	defer func() {
		if turnIndex <= 0 {
			return
		}
		data := map[string]any{
			"stop_reason": string(turnStop),
			"is_error":    turnError,
			"duration_ms": time.Since(turnStartedAt).Milliseconds(),
		}
		if lastContextUsage != nil {
			data["context_usage"] = lastContextUsage
		}
		e.emitTrace(turnIndex, "turn.finish", client.Provider(), "", "", data)
	}()

	lastStop := llm.StopReasonUnknown
	lastToolCalls := 0
	persistPolicy := classifyPersistencePolicy(cfg)
	persistState := persistenceState{}
	repeatGuard := newRepeatedToolCallGuard(repeatedToolCallLimit)
	evidenceActive := evidenceToolExposureEnabled(cfg, registry)
	evidenceProgress := evidenceState{}
	evidenceSelectedTools := make(map[string]bool)
	if evidenceActive {
		e.mu.RLock()
		msgs := e.messages
		e.mu.RUnlock()
		prefillEvidenceFromHistory(ctx, &evidenceProgress, msgs, registry)
	}
	contextWarnEmitted := false

	for iter := 0; iter < cfg.MaxToolIterations; iter++ {
		// toolCtx는 sysBlocks(SystemGuides) 조립에도 필요하므로 먼저 구성한다.
		toolCtx := tool.Context{
			Context:         ctx,
			State:           appState,
			WorkingDir:      deps.WorkingDir,
			Workspace:       deps.Workspace,
			ShellJobs:       deps.ShellJobs,
			Registry:        registry,
			ExecuteSubQuery: e.ExecuteSubQuery,
			Caps:            buildModelCaps(client, appState),
			Mode:            buildContextMode(appState),
			MCPEnabled:      registry.HasMCPTools(),
			Workspaces:      tool.BuildWorkspaceSummary(deps.Workspace),
		}

		e.mu.RLock()
		guideBlocks := registry.SystemGuides(toolCtx)
		sysBlocks := JoinSystemBlocks(systemFn(), workspaceSystemBlocks(deps.Workspace), inventorySystemBlocks(deps.Workspace), laneSystemBlocks(deps.Workspace))
		sysBlocks = append(sysBlocks, guideBlocks...)
		systemTokens := estimateSystemTokens(sysBlocks)
		contextWin := activeModelContextWindow(appState)
		if contextWin <= 0 {
			contextWin = cfg.ContextWindowFallback
		}
		messages := ctxpkg.RenderForLLM(e.graph, e.est, systemTokens, contextWin, currentUserText)
		e.mu.RUnlock()
		toolSpecs := registry.ToolSpecs(toolCtx)

		toolSpecs = filterToolSpecs(toolSpecs, appState)
		evidencePlan := evidencePlan{}
		var exposedToolNames map[string]bool
		if evidenceActive {
			evidencePlan = buildEvidencePlan(currentUserText, appState)
			toolSpecs = filterToolSpecsForEvidence(toolSpecs, registry, evidencePlan, evidenceSelectedTools)
			exposedToolNames = toolSpecNameSet(toolSpecs)
		}

		req := llm.Request{
			System:    sysBlocks,
			Messages:  messages,
			Tools:     toolSpecs,
			MaxTokens: cfg.MaxTokens,
		}

		estimatedTokens := 0
		if tokens, err := client.CountTokens(ctx, req); err == nil {
			estimatedTokens = tokens
			if contextWin > 0 {
				if appState != nil {
					pct := (tokens * 100) / contextWin
					appState.SetContextUsedPercent(pct)
					if pct >= 70 && !contextWarnEmitted {
						contextWarnEmitted = true
						out <- UIEvent{Kind: UIEventNotice, Notice: fmt.Sprintf("⚠ 컨텍스트 %d%% 사용 중 (%d / %d 토큰). 세션 분리를 고려하세요.", pct, tokens, contextWin)}
						e.emitTrace(turnIndex, "notice", client.Provider(), "", "", map[string]any{
							"category": "context_warning",
							"message":  fmt.Sprintf("컨텍스트 %d%% 사용 중 (%d / %d 토큰)", pct, tokens, contextWin),
						})
					}
				}
				if tokens >= contextWin {
					beforeTokens := tokens
					e.mu.Lock()
					e.graph.ForceConsolidate("[이전 대화 내용이 컨텍스트 최적화를 위해 요약되었습니다.]")
					e.mu.Unlock()
					e.emitTrace(turnIndex, "context.consolidate", client.Provider(), "", "", map[string]any{
						"trigger":       "token_limit",
						"before_tokens": beforeTokens,
						"context_win":   contextWin,
					})
					e.mu.RLock()
					messages = ctxpkg.RenderForLLM(e.graph, e.est, systemTokens, contextWin, currentUserText)
					e.mu.RUnlock()
					req.Messages = messages
					if newTokens, err := client.CountTokens(ctx, req); err == nil {
						tokens = newTokens
					}
				}
				remaining := contextWin - tokens
				if remaining <= 0 {
					remaining = 256
				}
				if req.MaxTokens > remaining {
					req.MaxTokens = remaining
				}
			}
			e.budget.AddInput(tokens)
		}
		if turnIndex > 0 {
			req.TraceHook = func(evt llm.TraceEvent) {
				switch evt.Kind {
				case llm.TraceKindRequest:
					usage := buildContextUsageBreakdown(req.System, req.Messages, req.Tools, estimatedTokens)
					lastContextUsage = usage
					data := map[string]any{
						"normalized":     sanitizeRequestForTrace(evt.Request),
						"token_estimate": estimatedTokens,
						"tool_count":     len(req.Tools),
						"context_usage":  usage,
					}
					if len(evt.Raw) > 0 {
						data["raw"] = json.RawMessage(evt.Raw)
					}
					e.emitTrace(turnIndex, "llm.request", evt.Provider, evt.Model, "", data)
				case llm.TraceKindProviderChunk:
					data := map[string]any{
						"mapped_kind":      string(evt.MappedKind),
						"mapped_delta":     sanitizeTextWithSecrets(evt.MappedDelta, nil),
						"mapped_tool_name": evt.MappedToolName,
					}
					if len(evt.Raw) > 0 {
						data["raw"] = json.RawMessage(evt.Raw)
					}
					if evt.MappedStop != nil {
						data["mapped_stop_reason"] = string(*evt.MappedStop)
					}
					e.emitTrace(turnIndex, "llm.provider_chunk", evt.Provider, evt.Model, "", data)
				}
			}
		}

		stream, err := client.Stream(ctx, req)
		if err != nil {
			turnError = true
			e.emitTrace(turnIndex, "error", client.Provider(), "", "", map[string]any{
				"stage": "llm.stream.start",
				"error": err.Error(),
			})
			out <- UIEvent{Kind: UIEventError, Err: err}
			return
		}

		var assistantText strings.Builder
		toolCalls := make([]llm.ToolUseStart, 0, 2)
		stopReason := llm.StopReasonUnknown
		var streamErr error
		thinkingOpen := false
		bufferAssistantText := shouldBufferPersistenceAssistantText(persistPolicy, persistState) ||
			(evidenceActive && shouldBufferEvidenceAssistantText(evidencePlan, evidenceProgress))
		subQueryFn, searchFn, fetchFn := e.classificationFunctions(ctx, turnIndex)
		streamingTools := tools.NewStreamingToolExecutor(ctx, registry, e.hookRegistry, hookDispatcher, func(tCtx context.Context, call llm.ToolUseStart) (string, bool) {
			if evidenceActive && !isToolExposed(call.Name, exposedToolNames) {
				msg := hiddenToolCallMessage(call.Name)
				e.emitTrace(turnIndex, "error", client.Provider(), "", call.ID, map[string]any{
					"stage": "tool.evidence_exposure_block",
					"error": msg,
					"tool":  call.Name,
				})
				return msg, true
			}
			if evidenceActive {
				if blocked, msg := evidenceBlocksPrematureMutation(tCtx, call, evidencePlan, evidenceProgress, registry, subQueryFn, searchFn, fetchFn); blocked {
					e.emitTrace(turnIndex, "error", client.Provider(), "", call.ID, map[string]any{
						"stage": "tool.evidence_prerequisite_block",
						"error": msg,
						"tool":  call.Name,
					})
					return msg, true
				}
			}
			out <- UIEvent{Kind: UIEventNotice, Notice: fmt.Sprintf("executing tool: %s", call.Name)}
			e.emitTrace(turnIndex, "notice", client.Provider(), "", call.ID, map[string]any{
				"category": "tool",
				"message":  fmt.Sprintf("executing tool: %s", call.Name),
			})
			autoApprove := false
			if appState != nil {
				autoApprove = appState.InYoloMode()
			}
			return e.invokeTool(tCtx, out, call, registry, appState, deps, autoApprove, turnIndex)
		})

	streamLoop:
		for evt := range stream {
			switch evt.Kind {
			case llm.EventThinkingDelta:
				if evt.Delta != "" {
					thinkingOpen = true
					e.budget.AddThinking(max(1, len([]rune(evt.Delta))/4))
					in, outTokens, think := e.budget.Snapshot()
					out <- UIEvent{
						Kind:           UIEventThinkingDelta,
						Delta:          evt.Delta,
						InputTokens:    in,
						OutputTokens:   outTokens,
						ThinkingTokens: think,
					}
				}
			case llm.EventTextDelta:
				if evt.Delta != "" {
					if thinkingOpen {
						thinkingOpen = false
						out <- UIEvent{Kind: UIEventThinkingDone}
						e.emitTrace(turnIndex, "llm.thinking", client.Provider(), "", "", map[string]any{
							"done": true,
						})
					}
					e.budget.AddOutput(max(1, len([]rune(evt.Delta))/4))
					assistantText.WriteString(evt.Delta)
					in, outTokens, think := e.budget.Snapshot()
					if bufferAssistantText {
						persistState.BufferedAssistantText += evt.Delta
					} else {
						out <- UIEvent{
							Kind:           UIEventAssistantDelta,
							Delta:          evt.Delta,
							InputTokens:    in,
							OutputTokens:   outTokens,
							ThinkingTokens: think,
						}
					}
				}
			case llm.EventToolUseStart:
				if evt.ToolUse != nil {
					if thinkingOpen {
						thinkingOpen = false
						out <- UIEvent{Kind: UIEventThinkingDone}
						e.emitTrace(turnIndex, "llm.thinking", client.Provider(), "", "", map[string]any{
							"done": true,
						})
					}
					safeToolUse := sanitizeToolUseForDisplay(*evt.ToolUse)
					e.emitTrace(turnIndex, "llm.tool_use", client.Provider(), "", evt.ToolUse.ID, map[string]any{
						"name":  evt.ToolUse.Name,
						"input": json.RawMessage(safeToolUse.Input),
					})
					toolCalls = append(toolCalls, *evt.ToolUse)
					// Shell tools are displayed only after Shell Guard has
					// normalized the command and selected the execution context.
					if !isShellTool(evt.ToolUse.Name) {
						out <- UIEvent{
							Kind:    UIEventToolUse,
							ToolUse: &safeToolUse,
							TaskID:  strings.TrimSpace(evt.ToolUse.ID),
						}
					}
					streamingTools.Add(*evt.ToolUse)
				}
			case llm.EventUsage:
				if evt.Usage != nil {
					u := evt.Usage
					e.emitTrace(turnIndex, "llm.usage", client.Provider(), "", "", map[string]any{
						"input_tokens":                u.InputTokens,
						"output_tokens":               u.OutputTokens,
						"cache_creation_input_tokens": u.CacheCreationInputTokens,
						"cache_read_input_tokens":     u.CacheReadInputTokens,
					})
				}
			case llm.EventStop:
				if evt.Stop != nil {
					stopReason = *evt.Stop
				}
			case llm.EventError:
				streamErr = evt.Err
				if thinkingOpen {
					thinkingOpen = false
					out <- UIEvent{Kind: UIEventThinkingDone}
					e.emitTrace(turnIndex, "llm.thinking", client.Provider(), "", "", map[string]any{
						"done": true,
					})
				}
				turnError = true
				errMsg := "unknown stream error"
				if evt.Err != nil {
					errMsg = evt.Err.Error()
				}
				e.emitTrace(turnIndex, "error", client.Provider(), "", "", map[string]any{
					"stage": "llm.stream.read",
					"error": errMsg,
				})
				out <- UIEvent{Kind: UIEventError, Err: evt.Err}
				break streamLoop
			}
		}
		if thinkingOpen {
			thinkingOpen = false
			out <- UIEvent{Kind: UIEventThinkingDone}
			e.emitTrace(turnIndex, "llm.thinking", client.Provider(), "", "", map[string]any{
				"done": true,
			})
		}
		if streamErr != nil {
			if len(toolCalls) > 0 {
				_ = streamingTools.CloseAndWait()
			}
			turnStop = llm.StopReasonUnknown
			e.callStopHooks(ctx, stopHooks, llm.StopReasonUnknown, 0)
			return
		}

		if stopReason == llm.StopReasonUnknown {
			stopReason = llm.StopReasonEndTurn
		}
		lastStop = stopReason
		lastToolCalls = len(toolCalls)
		turnStop = stopReason

		assistantTextValue := assistantText.String()
		assistantFinalText, assistantFinalDelta := assistantFinalTextForStop(assistantTextValue, stopReason)
		recordAssistant := true
		recordedAssistantText := assistantFinalText
		if bufferAssistantText {
			if len(toolCalls) == 0 {
				recordAssistant = false
			} else {
				recordedAssistantText = ""
			}
		}

		if recordAssistant {
			safeToolCalls := sanitizeToolCallsForStorage(toolCalls)
			e.mu.Lock()
			e.messages = append(e.messages, newAssistantMessage(recordedAssistantText, safeToolCalls))
			if strings.TrimSpace(recordedAssistantText) != "" {
				e.graph.AppendAssistant(recordedAssistantText)
			}
			for _, tc := range safeToolCalls {
				e.graph.AppendToolUse(tc.ID, tc.Name, tc.Input)
			}
			e.mu.Unlock()
		}

		if len(toolCalls) == 0 {
			persistState.NoteAssistantTurnText(assistantTextValue)
			if shouldForcePersistenceContinuation(persistPolicy, persistState) {
				persistState.ForcedContinuations++
				persistState.Web.ForcedRetries++
				followUp := buildPersistenceFollowUpPrompt(persistPolicy, persistState)
				e.mu.Lock()
				e.messages = append(e.messages, newUserTextMessage(followUp))
				e.graph.AppendUser(followUp)
				e.mu.Unlock()
				out <- UIEvent{Kind: UIEventNotice, Notice: "continuing turn for persistence checks"}
				e.emitTrace(turnIndex, "notice", client.Provider(), "", "", map[string]any{
					"category": "persistence",
					"message":  "continuing turn for persistence checks",
				})
				continue
			}
			if evidenceActive && shouldForceEvidenceContinuation(evidencePlan, evidenceProgress, cfg) {
				evidenceProgress.ForcedContinuations++
				followUp := buildEvidenceFollowUpPrompt(evidencePlan, evidenceProgress)
				e.mu.Lock()
				e.messages = append(e.messages, newUserTextMessage(followUp))
				e.graph.AppendUser(followUp)
				e.mu.Unlock()
				out <- UIEvent{Kind: UIEventNotice, Notice: "continuing turn for evidence checks"}
				e.emitTrace(turnIndex, "notice", client.Provider(), "", "", map[string]any{
					"category": "evidence",
					"message":  "continuing turn for evidence checks",
				})
				continue
			}
			if shouldBufferPersistenceAssistantText(persistPolicy, persistState) {
				failure := buildPersistenceFailureMessage(persistPolicy, persistState)
				e.finishAssistantFinal(ctx, out, client, turnIndex, stopHooks, stopReason, len(toolCalls), failure, true, failure)
				return
			}
			if evidenceActive && shouldBufferEvidenceAssistantText(evidencePlan, evidenceProgress) {
				failure := buildEvidenceFailureMessage(evidencePlan, evidenceProgress)
				e.finishAssistantFinal(ctx, out, client, turnIndex, stopHooks, stopReason, len(toolCalls), failure, true, failure)
				return
			}
			e.finishAssistantFinal(ctx, out, client, turnIndex, stopHooks, stopReason, len(toolCalls), assistantFinalText, false, assistantFinalDelta)
			return
		}

		for _, batch := range tools.PartitionToolCalls(toolCalls, registry) {
			if batch.IsConcurrencySafe && len(batch.Calls) > 1 {
				ids := make([]string, len(batch.Calls))
				for i, c := range batch.Calls {
					ids[i] = strings.TrimSpace(c.ID)
				}
				out <- UIEvent{Kind: UIEventParallelBatch, BatchTaskIDs: ids}
			}
		}

		results := streamingTools.CloseAndWait()

		for _, res := range results {
			persistState.ObserveToolResult(res.Call, res.Output, res.IsError, registry)
			if evidenceActive {
				evidenceProgress.ObserveToolResult(ctx, res.Call, res.IsError, registry, subQueryFn, searchFn, fetchFn)
				if tool.CanonicalName(res.Call.Name) == "tool_search" && !res.IsError {
					for _, name := range parseToolSearchResultNames(res.Output) {
						evidenceSelectedTools[tool.CanonicalName(name)] = true
					}
				}
			}
			if steps := parsePlanExecutionReady(res.Call.Name, res.Output, res.IsError); len(steps) > 0 {
				out <- UIEvent{
					Kind:      UIEventPlanExecutionReady,
					PlanSteps: steps,
				}
			}
			e.appendToolResult(res.Call.ID, res.Call.Name, res.Output, res.IsError)
			resultTaskID := extractOutputTaskID(res.Output)
			if resultTaskID == "" {
				resultTaskID = strings.TrimSpace(res.Call.ID)
			}
			out <- UIEvent{
				Kind:       UIEventToolResult,
				ToolName:   res.Call.Name,
				ToolResult: res.Output,
				TaskID:     resultTaskID,
			}

			if strings.EqualFold(res.Call.Name, "snip") && !res.IsError {
				e.mu.Lock()
				e.graph.MarkProtected(currentUserText)
				e.graph.ForceConsolidate("[사용자 요청으로 이전 대화가 최적화되었습니다.]")
				e.mu.Unlock()
				e.emitTrace(turnIndex, "context.consolidate", "", "", "", map[string]any{
					"trigger": "snip",
				})
				out <- UIEvent{Kind: UIEventNotice, Notice: "history snipped for optimization"}
				e.emitTrace(turnIndex, "notice", "", "", "", map[string]any{
					"category": "context",
					"message":  "history snipped for optimization",
				})
			}
		}

		if repeated, count, sig := repeatGuard.Observe(toolCalls); repeated {
			turnError = true
			turnStop = normalizeFinalStopReason(lastStop)
			msg := fmt.Sprintf("repeated identical tool calls detected (%d times): %s. Stopping this turn to avoid a tool loop.", count, summarizeToolCallSignature(sig))
			e.emitTrace(turnIndex, "error", client.Provider(), "", "", map[string]any{
				"stage":     "engine.repeated_tool_calls",
				"error":     msg,
				"signature": summarizeToolCallSignature(sig),
				"count":     count,
			})
			e.finishAssistantFinal(ctx, out, client, turnIndex, stopHooks, lastStop, lastToolCalls, msg, true, msg)
			out <- UIEvent{
				Kind:       UIEventError,
				Err:        errors.New(msg),
				StopReason: lastStop,
			}
			return
		}
	}

	if deps.Workspace != nil && e.state != nil {
		reg := deps.Workspace.Registry()
		var ephemeral []interface{}
		for _, entry := range reg.List() {
			if entry.IsEphemeral {
				ephemeral = append(ephemeral, entry)
			}
		}
		e.state.SetEphemeralServers(ephemeral)
	}

	turnStop = normalizeFinalStopReason(lastStop)
	turnError = true
	e.emitTrace(turnIndex, "error", client.Provider(), "", "", map[string]any{
		"stage": "engine.max_tool_iterations",
		"error": fmt.Sprintf("max tool iterations reached (%d)", cfg.MaxToolIterations),
	})
	msg := fmt.Sprintf("max tool iterations reached (%d). Stopping this turn to avoid an endless tool loop.", cfg.MaxToolIterations)
	e.finishAssistantFinal(ctx, out, client, turnIndex, stopHooks, lastStop, lastToolCalls, msg, true, msg)
	out <- UIEvent{
		Kind:       UIEventError,
		Err:        fmt.Errorf("max tool iterations reached (%d)", cfg.MaxToolIterations),
		StopReason: lastStop,
	}
}

func assistantFinalTextForStop(text string, stop llm.StopReason) (string, string) {
	if stop != llm.StopReasonMaxTokens {
		return text, ""
	}
	const notice = "[Argus] Output token limit reached before the model completed a final answer. The response above may be incomplete."
	if strings.Contains(text, notice) {
		return text, ""
	}
	if strings.TrimSpace(text) == "" {
		return notice, notice
	}
	delta := "\n\n" + notice
	return strings.TrimRight(text, "\r\n") + delta, delta
}

func normalizeFinalStopReason(stop llm.StopReason) llm.StopReason {
	if stop == llm.StopReasonUnknown {
		return llm.StopReasonEndTurn
	}
	return stop
}

func (e *Engine) finishAssistantFinal(
	ctx context.Context,
	out chan<- UIEvent,
	client llm.LLM,
	turnIndex int,
	stopHooks []StopHook,
	stop llm.StopReason,
	toolCalls int,
	text string,
	recordMessage bool,
	delta string,
) {
	stop = normalizeFinalStopReason(stop)
	if recordMessage {
		e.mu.Lock()
		e.messages = append(e.messages, newAssistantMessage(text, nil))
		if strings.TrimSpace(text) != "" {
			e.graph.AppendAssistant(text)
		}
		e.mu.Unlock()
	}
	if strings.TrimSpace(delta) != "" {
		out <- UIEvent{Kind: UIEventAssistantDelta, Delta: delta}
	}
	e.emitTrace(turnIndex, "assistant.final", client.Provider(), "", "", map[string]any{
		"text":        text,
		"chars":       len(text),
		"stop_reason": string(stop),
	})
	out <- UIEvent{Kind: UIEventDone, StopReason: stop}
	e.callStopHooks(ctx, stopHooks, stop, toolCalls)
}

func (e *Engine) invokeTool(
	ctx context.Context,
	out chan<- UIEvent,
	call llm.ToolUseStart,
	registry *tool.Registry,
	appState *state.AppState,
	deps Deps,
	autoApproveAsk bool,
	turnIndex int,
) (string, bool) {
	toolImpl, ok := registry.Lookup(call.Name)
	if !ok {
		e.emitTrace(turnIndex, "error", "", "", call.ID, map[string]any{
			"stage": "tool.lookup",
			"error": fmt.Sprintf("tool not found: %s", call.Name),
		})
		return fmt.Sprintf("tool not found: %s", call.Name), true
	}

	if appState != nil && appState.InPlanMode() && !toolImpl.IsReadOnly() && !isPlanModeWriteException(call.Name) {
		e.emitTrace(turnIndex, "error", "", "", call.ID, map[string]any{
			"stage": "tool.plan_mode_block",
			"error": fmt.Sprintf("tool blocked in plan mode: %s", call.Name),
		})
		return fmt.Sprintf("tool blocked in plan mode: %s", call.Name), true
	}

	if !autoApproveAsk {
		if blocked, reason := ValidateWorkflowStep(appState, call.Name, call.Input, toolImpl.IsReadOnly()); blocked {
			e.emitTrace(turnIndex, "error", "", "", call.ID, map[string]any{
				"stage": "tool.workflow_block",
				"error": reason,
				"tool":  call.Name,
			})
			return reason, true
		}
	}

	toolCtx := tool.Context{
		Context:         ctx,
		State:           appState,
		WorkingDir:      deps.WorkingDir,
		Workspace:       deps.Workspace,
		ShellJobs:       deps.ShellJobs,
		Registry:        registry,
		ExecuteSubQuery: e.ExecuteSubQuery,
		EmitTrace:       e.EmitTrace,
	}

	if isShellTool(call.Name) {
		prepared, blocked, reason := e.prepareShellToolCall(ctx, call, toolCtx, turnIndex)
		call = prepared
		safeToolUse := sanitizeToolUseForDisplay(call)
		if out != nil {
			out <- UIEvent{
				Kind:    UIEventToolUse,
				ToolUse: &safeToolUse,
				TaskID:  strings.TrimSpace(call.ID),
			}
		}
		if blocked {
			e.emitTrace(turnIndex, "error", "", "", call.ID, map[string]any{
				"stage": "tool.shell_guard",
				"error": reason,
				"tool":  call.Name,
			})
			return reason, true
		}
	}
	toolSecrets := extractToolSecrets(call.Input)
	safeInput := sanitizeToolInput(call.Input)

	var (
		perm tool.PermissionResult
		err  error
	)
	if preApproved := e.findPreApprovedRule(appState, call.Name, safeInput); preApproved != nil {
		perm = tool.PermissionResult{
			Behavior: types.BehaviorAllow,
			DecisionReason: &types.PermissionDecisionReason{
				Type: types.DecisionReasonRule,
				Rule: preApproved,
			},
		}
	} else {
		perm, err = toolImpl.CheckPermission(toolCtx, call.Input)
		if err != nil {
			e.emitTrace(turnIndex, "tool.permission", "", "", call.ID, map[string]any{
				"tool":       call.Name,
				"behavior":   "error",
				"is_allowed": false,
				"error":      err.Error(),
			})
			return fmt.Sprintf("permission check failed: %v", err), true
		}
	}

	allowed := perm.Behavior == types.BehaviorAllow
	if perm.Behavior == types.BehaviorPassthrough {
		if toolImpl.IsReadOnly() {
			allowed = true
		} else {
			perm.Behavior = types.BehaviorAsk
		}
	}
	if perm.Behavior == types.BehaviorAsk {
		if autoApproveAsk {
			allowed = true
		} else if deps.ApproveTool.Prompt != nil {
			ok, err := deps.ApproveTool.Prompt(ctx, call.Name, cloneJSON(safeInput))
			if err != nil {
				allowed = false
			} else {
				allowed = ok
			}
		} else {
			allowed = false
		}
	}
	e.emitTrace(turnIndex, "tool.permission", "", "", call.ID, map[string]any{
		"tool":       call.Name,
		"behavior":   string(perm.Behavior),
		"is_allowed": allowed,
		"auto":       autoApproveAsk,
	})

	if !allowed {
		msg := perm.Message
		if msg == "" {
			msg = fmt.Sprintf("permission denied for %s", call.Name)
		}
		e.emitTrace(turnIndex, "error", "", "", call.ID, map[string]any{
			"stage": "tool.permission",
			"error": msg,
		})
		return msg, true
	}

	startedAt := time.Now()
	traceStartData := map[string]any{
		"tool":  call.Name,
		"input": json.RawMessage(safeInput),
	}
	if servers := extractTraceServers(call.Name, safeInput); len(servers) > 0 {
		traceStartData["servers"] = servers
	}
	e.emitTrace(turnIndex, "tool.call.start", "", "", call.ID, traceStartData)
	resultStream, err := toolImpl.Call(toolCtx, call.Input)
	if err != nil {
		e.emitTrace(turnIndex, "tool.call.finish", "", "", call.ID, map[string]any{
			"tool":        call.Name,
			"is_error":    true,
			"duration_ms": time.Since(startedAt).Milliseconds(),
			"error":       err.Error(),
		})
		e.emitTrace(turnIndex, "error", "", "", call.ID, map[string]any{
			"stage": "tool.call.start",
			"error": err.Error(),
		})
		return fmt.Sprintf("tool call failed: %v", err), true
	}

	var toolOut strings.Builder
	isError := false
	for te := range resultStream {
		switch te.Kind {
		case tool.ToolEventChunk:
			safeOutput := sanitizeTextWithSecrets(te.Output, toolSecrets)
			if out != nil && (isShellTool(call.Name) || te.InputResponse != nil) {
				out <- UIEvent{
					Kind:          UIEventToolDelta,
					Delta:         safeOutput,
					ToolName:      call.Name,
					TaskID:        strings.TrimSpace(call.ID),
					InputResponse: te.InputResponse,
				}
			}
		case tool.ToolEventOutput:
			safeOutput := sanitizeTextWithSecrets(te.Output, toolSecrets)
			toolOut.WriteString(safeOutput)
			e.emitTrace(turnIndex, "tool.call.output", "", "", call.ID, map[string]any{
				"tool":   call.Name,
				"kind":   string(te.Kind),
				"output": safeOutput,
			})
		case tool.ToolEventPasswordPrompt:
			e.emitTrace(turnIndex, "tool.call.output", "", "", call.ID, map[string]any{
				"tool":   call.Name,
				"kind":   string(te.Kind),
				"prompt": sanitizeTextWithSecrets(te.Prompt, toolSecrets),
			})
			if out != nil {
				out <- UIEvent{
					Kind:             UIEventPasswordPrompt,
					ToolName:         call.Name,
					TaskID:           strings.TrimSpace(call.ID),
					Prompt:           sanitizeTextWithSecrets(te.Prompt, toolSecrets),
					PasswordResponse: te.PasswordResponse,
				}
			}
		case tool.ToolEventAskUserPrompt:
			e.emitTrace(turnIndex, "tool.call.output", "", "", call.ID, map[string]any{
				"tool":     call.Name,
				"kind":     string(te.Kind),
				"question": sanitizeTraceData(te.Question, toolSecrets),
			})
			if out != nil {
				out <- UIEvent{
					Kind:            UIEventAskUserPrompt,
					ToolName:        call.Name,
					TaskID:          strings.TrimSpace(call.ID),
					Question:        te.Question,
					AskUserResponse: te.AskUserResponse,
				}
			}
		case tool.ToolEventAskUserBatchPrompt:
			e.emitTrace(turnIndex, "tool.call.output", "", "", call.ID, map[string]any{
				"tool":      call.Name,
				"kind":      string(te.Kind),
				"questions": sanitizeTraceData(te.AskUserBatchPrompt, toolSecrets),
			})
			if out != nil {
				batch := []tool.AskUserQuestion(nil)
				if te.AskUserBatchPrompt != nil {
					batch = append(batch, te.AskUserBatchPrompt.Questions...)
				}
				out <- UIEvent{
					Kind:                 UIEventAskUserBatchPrompt,
					ToolName:             call.Name,
					TaskID:               strings.TrimSpace(call.ID),
					Questions:            batch,
					AskUserBatchResponse: te.AskUserBatchResponse,
				}
			}
		case tool.ToolEventError:
			isError = true
			if te.Err != nil {
				safeErr := sanitizeTextWithSecrets(te.Err.Error(), toolSecrets)
				toolOut.WriteString(safeErr)
				e.emitTrace(turnIndex, "tool.call.output", "", "", call.ID, map[string]any{
					"tool":   call.Name,
					"kind":   string(te.Kind),
					"output": safeErr,
				})
			}
		}
	}

	text := sanitizeTextWithSecrets(strings.TrimSpace(toolOut.String()), toolSecrets)
	if text == "" {
		text = "(no output)"
	}
	e.emitTrace(turnIndex, "tool.call.finish", "", "", call.ID, map[string]any{
		"tool":         call.Name,
		"is_error":     isError,
		"duration_ms":  time.Since(startedAt).Milliseconds(),
		"output_chars": len(text),
		"output":       text,
	})

	e.mu.RLock()
	dist := e.distiller
	seq := e.graph.Len() + 1
	e.mu.RUnlock()

	if dist != nil {
		dr := dist.Distill(seq, call.Name, call.ID, text, isError)
		if dr.Projection != ctxpkg.ProjectionFull && dr.ArtifactPath != "" && !strings.Contains(dr.InlineText, dr.ArtifactPath) {
			return fmt.Sprintf("%s\n\n[전체 출력: %s]", dr.InlineText, dr.ArtifactPath), isError
		}
		return dr.InlineText, isError
	}

	text = ctxpkg.NormalizeToolResultForContext(call.Name, text)
	maxChars := toolImpl.MaxResultSizeChars()
	if maxChars > 0 && len(text) > maxChars {
		truncated := text[:maxChars]
		text = fmt.Sprintf("%s\n\n... [%d characters truncated] ...", truncated, len(text)-maxChars)
	}
	return text, isError
}

func (e *Engine) appendToolResult(toolUseID, toolName, text string, isError bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	safeText := sanitizeTextWithSecrets(text, nil)
	e.messages = append(e.messages, newToolResultMessage(toolUseID, toolName, safeText, isError))
	e.graph.AppendToolResult(toolUseID, toolName, safeText, ctxpkg.ProjectionFull, isError, "", "")
}

func (e *Engine) callStopHooks(ctx context.Context, stopHooks []StopHook, stop llm.StopReason, toolCalls int) {
	if d := e.hookDispatcher; d != nil && !d.IsEmpty() {
		d.Dispatch(ctx, types.HookEventStop, hooks.HookInput{})
	}

	if len(stopHooks) == 0 {
		return
	}
	messageCount := len(e.Messages())
	summary := StopSummary{
		StopReason: stop,
		ToolCalls:  toolCalls,
		Messages:   messageCount,
	}
	for _, h := range stopHooks {
		h(ctx, summary)
	}
}

// EmitTrace는 engine 외부(main.go 등)에서 aidebug trace를 발행할 때 사용합니다.
func (e *Engine) EmitTrace(traceType, callID string, data any) {
	e.mu.RLock()
	turnIndex := e.debugTurn
	provider := ""
	if e.llm != nil {
		provider = e.llm.Provider()
	}
	e.mu.RUnlock()
	e.emitTrace(turnIndex, traceType, provider, modelForTrace(e.llm), callID, data)
}

func (e *Engine) emitTrace(turnIndex int, traceType, provider, model, callID string, data any) {
	if turnIndex <= 0 {
		return
	}
	e.mu.Lock()
	deps := e.deps
	if !deps.AIDebug.Enabled || deps.AIDebug.Emitter != nil {
		// logic continues below
	} else {
		e.mu.Unlock()
		return
	}
	e.debugSeq++
	processedData := truncateLargeData(sanitizeTraceData(data, nil))
	record := TraceRecord{
		TS:        time.Now().UTC().Format(time.RFC3339Nano),
		Type:      traceType,
		SessionID: deps.SessionID,
		TurnIndex: turnIndex,
		Seq:       e.debugSeq,
		Provider:  strings.TrimSpace(provider),
		Model:     strings.TrimSpace(model),
		CallID:    strings.TrimSpace(callID),
		Data:      processedData,
	}
	emitter := deps.AIDebug.Emitter
	e.mu.Unlock()
	if emitter != nil {
		emitter.Emit(record)
	}
}

func modelForTrace(l llm.LLM) string {
	if l == nil {
		return ""
	}
	return ""
}
