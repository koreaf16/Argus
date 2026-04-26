package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/koreaf16/argus/internal/constants"
	ctxpkg "github.com/koreaf16/argus/internal/context"
	"github.com/koreaf16/argus/internal/hooks"
	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/services/tools"
	"github.com/koreaf16/argus/internal/state"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
	permutils "github.com/koreaf16/argus/internal/utils/permissions"
)

type Engine struct {
	mu sync.RWMutex

	llm            llm.LLM
	registry       *tool.Registry
	hookRegistry   *tools.HookRegistry
	hookDispatcher *hooks.HookDispatcher
	fileWatcher    *hooks.FileWatcher
	state          *state.AppState
	messages       []llm.Message // legacy: snapshot load/save 호환용으로 유지
	systemFn       func() []llm.SystemBlock

	cfg       Config
	deps      Deps
	stopHooks []StopHook
	budget    *TokenBudget

	// --- context management (graph-based) ---
	graph     *ctxpkg.Graph
	est       *ctxpkg.TokenEstimator
	distiller *ctxpkg.Distiller
	artStore  *ctxpkg.ArtifactStore
	artMF     *ctxpkg.ArtifactManifest

	debugSeq            int
	debugTurn           int
	debugSessionStarted bool
	debugSessionID      string
	hookSessionStartKey string

	permissionRulesCache      []types.PermissionRule
	permissionRulesCachedAt   time.Time
	permissionRulesCacheValid bool
}

const permissionRuleCacheTTL = 2 * time.Second

func NewEngine(client llm.LLM, registry *tool.Registry, appState *state.AppState, systemFn func() []llm.SystemBlock) *Engine {
	if systemFn == nil {
		systemFn = DefaultSystemPrompt
	}
	if registry == nil {
		registry = tool.NewRegistry()
	}
	if appState == nil {
		appState = state.NewAppState()
	}
	mf := ctxpkg.NewArtifactManifest()
	eg := &Engine{
		llm:          client,
		registry:     registry,
		hookRegistry: tools.NewHookRegistry(),
		state:        appState,
		messages:     make([]llm.Message, 0, 32),
		systemFn:     systemFn,
		cfg:          DefaultConfig(),
		budget:       NewTokenBudget(),
		graph:        ctxpkg.NewGraph(),
		est:          ctxpkg.NewTokenEstimator(),
		artMF:        mf,
	}
	// ArtifactStore는 SetDeps()로 baseDir/sessionID가 설정된 후 초기화됨
	return eg
}

func (e *Engine) SetLLM(client llm.LLM) {
	e.mu.Lock()
	e.llm = client
	e.mu.Unlock()
}

func (e *Engine) SetConfig(cfg Config) {
	e.mu.Lock()
	e.cfg = cfg
	e.mu.Unlock()
}

func (e *Engine) SetDeps(ctx context.Context, deps Deps) {
	var (
		sessionStart       *TraceRecord
		watcherToStop      *hooks.FileWatcher
		dispatcher         *hooks.HookDispatcher
		dispatchSession    bool
		sessionStartInput  hooks.HookInput
		shouldStartWatcher bool
	)

	e.mu.Lock()
	watcherToStop = e.fileWatcher
	e.fileWatcher = nil
	e.deps = deps

	// ArtifactStore initialization (requires baseDir and sessionID).
	if deps.BaseDir != "" && deps.SessionID != "" {
		store := ctxpkg.NewArtifactStore(deps.BaseDir, deps.SessionID)
		_ = store.Bootstrap()
		e.artStore = store
		e.distiller = ctxpkg.NewDistiller(store, e.artMF, e.makeSummarizeFn())
	} else {
		e.artStore = nil
		e.distiller = nil
	}

	// Emit debug session start once per session id.
	if deps.AIDebug.Enabled && deps.AIDebug.Emitter != nil {
		sessionID := strings.TrimSpace(deps.SessionID)
		if sessionID != "" && sessionID != e.debugSessionID {
			e.debugSessionID = sessionID
			e.debugSessionStarted = true
			e.debugSeq++
			sessionStart = &TraceRecord{
				TS:        time.Now().UTC().Format(time.RFC3339Nano),
				Type:      "session.start",
				SessionID: deps.SessionID,
				Seq:       e.debugSeq,
				Data: map[string]any{
					"working_dir": deps.WorkingDir,
				},
			}
		}
	} else {
		e.debugSessionStarted = false
		e.debugSessionID = ""
	}

	dispatcher = e.hookDispatcher
	if dispatcher != nil && !dispatcher.IsEmpty() {
		key := strings.TrimSpace(deps.SessionID) + "|" + strings.TrimSpace(deps.WorkingDir)
		if key != e.hookSessionStartKey {
			e.hookSessionStartKey = key
			dispatchSession = true
			sessionStartInput = hooks.HookInput{
				SessionID:  deps.SessionID,
				WorkingDir: deps.WorkingDir,
			}
		}
		shouldStartWatcher = true
	}

	// Deps can affect permission decisions; drop stale cache.
	e.permissionRulesCacheValid = false
	e.permissionRulesCache = nil
	e.permissionRulesCachedAt = time.Time{}
	e.mu.Unlock()

	if watcherToStop != nil {
		watcherToStop.Stop()
	}
	if sessionStart != nil {
		deps.AIDebug.Emitter.Emit(*sessionStart)
	}
	if dispatcher == nil || dispatcher.IsEmpty() {
		return
	}
	if dispatchSession {
		go dispatcher.Dispatch(ctx, types.HookEventSessionStart, sessionStartInput)
	}
	if shouldStartWatcher {
		if fw, err := hooks.NewFileWatcher(ctx, dispatcher); err == nil && fw != nil {
			e.mu.Lock()
			e.fileWatcher = fw
			e.mu.Unlock()
			fw.Start()
		}
	}
}

func (e *Engine) SetApprovalGate(gate ApprovalGate) {
	e.mu.Lock()
	e.deps.ApproveTool = gate
	e.mu.Unlock()
}

func (e *Engine) InvalidatePermissionRuleCache() {
	e.mu.Lock()
	e.permissionRulesCache = nil
	e.permissionRulesCacheValid = false
	e.permissionRulesCachedAt = time.Time{}
	e.mu.Unlock()
}

// SetHookDispatcher configures the settings-based hook dispatcher.
func (e *Engine) SetHookDispatcher(d *hooks.HookDispatcher) {
	var watcherToStop *hooks.FileWatcher
	e.mu.Lock()
	watcherToStop = e.fileWatcher
	e.fileWatcher = nil
	e.hookDispatcher = d
	e.hookSessionStartKey = ""
	e.mu.Unlock()
	if watcherToStop != nil {
		watcherToStop.Stop()
	}
}

func clonePermissionRules(in []types.PermissionRule) []types.PermissionRule {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.PermissionRule, len(in))
	copy(out, in)
	return out
}

func (e *Engine) loadDiskPermissionRulesCached() []types.PermissionRule {
	now := time.Now()
	e.mu.RLock()
	cacheValid := e.permissionRulesCacheValid
	cachedAt := e.permissionRulesCachedAt
	cached := clonePermissionRules(e.permissionRulesCache)
	e.mu.RUnlock()
	if cacheValid && now.Sub(cachedAt) < permissionRuleCacheTTL {
		return cached
	}
	rules := permutils.LoadAllPermissionRulesFromDisk()
	e.mu.Lock()
	e.permissionRulesCache = clonePermissionRules(rules)
	e.permissionRulesCachedAt = now
	e.permissionRulesCacheValid = true
	e.mu.Unlock()
	return rules
}

func (e *Engine) findPreApprovedRule(appState *state.AppState, toolName string, input json.RawMessage) *types.PermissionRule {
	rules := e.loadDiskPermissionRulesCached()
	if appState != nil {
		rules = append(rules, appState.SessionPermissionRules()...)
	}
	if len(rules) == 0 {
		return nil
	}

	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return nil
	}
	command := strings.TrimSpace(tool.ExtractStringInput(input, "command"))
	isShellToolName := strings.EqualFold(toolName, "bash") || strings.EqualFold(toolName, "powershell")
	caseInsensitiveShell := strings.EqualFold(toolName, "powershell")

	for _, rule := range rules {
		if rule.RuleBehavior != types.BehaviorAllow {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(rule.RuleValue.ToolName), toolName) {
			continue
		}
		content := strings.TrimSpace(rule.RuleValue.RuleContent)
		if content == "" {
			matched := rule
			return &matched
		}
		if !isShellToolName || command == "" {
			continue
		}
		parsed := permutils.ParsePermissionRule(content)
		if permutils.MatchesPermissionRule(parsed, command, caseInsensitiveShell) {
			matched := rule
			return &matched
		}
	}
	return nil
}

func (e *Engine) setDepsLegacy(ctx context.Context, deps Deps) {
	var sessionStart *TraceRecord
	e.mu.Lock()
	e.deps = deps
	// ArtifactStore 초기화 (baseDir, sessionID 필요)
	if deps.BaseDir != "" && deps.SessionID != "" {
		store := ctxpkg.NewArtifactStore(deps.BaseDir, deps.SessionID)
		_ = store.Bootstrap()
		e.artStore = store
		e.distiller = ctxpkg.NewDistiller(store, e.artMF, e.makeSummarizeFn())
	}
	if deps.AIDebug.Enabled && deps.AIDebug.Emitter != nil && !e.debugSessionStarted {
		e.debugSeq++
		e.debugSessionStarted = true
		sessionStart = &TraceRecord{
			TS:        time.Now().UTC().Format(time.RFC3339Nano),
			Type:      "session.start",
			SessionID: deps.SessionID,
			Seq:       e.debugSeq,
			Data: map[string]any{
				"working_dir": deps.WorkingDir,
			},
		}
	}
	e.mu.Unlock()
	if sessionStart != nil {
		deps.AIDebug.Emitter.Emit(*sessionStart)
	}
	// SessionStart 훅 비동기 발화 (차단 없음) — 부모 ctx를 전달해 세션 종료 시 취소 가능
	if d := e.hookDispatcher; d != nil && !d.IsEmpty() {
		if d := e.hookDispatcher; d != nil && !d.IsEmpty() {
			go d.Dispatch(ctx, types.HookEventSessionStart, hooks.HookInput{
				SessionID:  deps.SessionID,
				WorkingDir: deps.WorkingDir,
			})
			// FileChanged 훅이 있으면 파일 감시 시작
			if fw, err := hooks.NewFileWatcher(ctx, d); err == nil && fw != nil {
				e.mu.Lock()
				e.fileWatcher = fw
				e.mu.Unlock()
				fw.Start()
			}
		}
	}
}

// SetHookDispatcher 는 settings.json 기반 훅 디스패처를 설정한다.
func (e *Engine) setHookDispatcherLegacy(d *hooks.HookDispatcher) {
	e.mu.Lock()
	e.hookDispatcher = d
	e.mu.Unlock()
}

func (e *Engine) AddStopHook(hook StopHook) {
	if hook == nil {
		return
	}
	e.mu.Lock()
	e.stopHooks = append(e.stopHooks, hook)
	e.mu.Unlock()
}

func (e *Engine) Messages() []llm.Message {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return cloneMessages(e.messages)
}

func (e *Engine) TokenSnapshot() (input, output int) {
	return e.budget.Snapshot()
}

func (e *Engine) ResetBudget() {
	e.budget.Reset()
}

func (e *Engine) ReplaceMessages(messages []llm.Message) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.messages = cloneMessages(messages)
	// Rebuild graph so RenderForLLM has history after a session restore.
	// Tool pairs are reconstructed from flat message content; ToolName is unknown
	// at this level but the text content and pair linkage are preserved.
	e.graph = ctxpkg.NewGraph()
	for _, m := range messages {
		switch m.Role {
		case llm.RoleUser:
			texts := make([]string, 0, len(m.Content))
			for _, cb := range m.Content {
				switch cb.Type {
				case llm.ContentText:
					if cb.Text != "" {
						texts = append(texts, cb.Text)
					}
				case llm.ContentToolResult:
					e.graph.AppendToolResult(cb.ToolUseID, "", cb.Text, ctxpkg.ProjectionFull, cb.IsError, "", "")
				}
			}
			if len(texts) > 0 {
				e.graph.AppendUser(strings.Join(texts, "\n"))
			}
		case llm.RoleAssistant:
			texts := make([]string, 0, len(m.Content))
			for _, cb := range m.Content {
				switch cb.Type {
				case llm.ContentText:
					if cb.Text != "" {
						texts = append(texts, cb.Text)
					}
				case llm.ContentToolUse:
					if len(texts) > 0 {
						e.graph.AppendAssistant(strings.Join(texts, "\n"))
						texts = texts[:0]
					}
					e.graph.AppendToolUse(cb.ID, cb.Name, cb.Input)
				}
			}
			if len(texts) > 0 {
				e.graph.AppendAssistant(strings.Join(texts, "\n"))
			}
		}
	}
}

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

	callInput, err := buildPlannedStepToolCallInput(toolName, prompt, deps.WorkingDir)
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
	e.mu.Unlock()

	// UserPromptSubmit 훅: 차단 시 빈 채널 반환
	if dispatcher != nil && !dispatcher.IsEmpty() {
		agg := dispatcher.Dispatch(ctx, types.HookEventUserPrompt, hooks.HookInput{
			UserPrompt: text,
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
	// graph에도 user 노드 추가
	if client == nil {
		e.mu.Unlock()
		return nil, fmt.Errorf("llm is not configured")
	}
	e.messages = append(e.messages, newUserTextMessage(text))
	// graph?먮룄 user ?몃뱶 異붽?
	e.graph.AppendUser(text)
	cfg := e.cfg
	systemFn := e.systemFn
	registry := e.registry
	appState := e.state
	deps := e.deps
	stopHooksCopy := append([]StopHook(nil), e.stopHooks...)
	hookDispatcher := e.hookDispatcher
	currentUserText := text
	recentConversation := buildRecentConversationHint(e.messages, 1200)
	turnIndex := 0
	if deps.AIDebug.Enabled && deps.AIDebug.Emitter != nil {
		e.debugTurn++
		turnIndex = e.debugTurn
	}
	e.mu.Unlock()

	if cfg.MaxToolIterations <= 0 {
		cfg.MaxToolIterations = 1
	}

	out := make(chan UIEvent, 64)
	go e.run(ctx, out, client, cfg, systemFn, registry, appState, deps, stopHooksCopy, hookDispatcher, currentUserText, recentConversation, turnIndex)
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
	recentConversation string,
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
	// [보강] 자동화 테스트를 위해 웹 검증 정책을 기본적으로 비활성화하거나 유연하게 처리
	webPolicy := webEvidencePolicy{Enabled: false} 
	webState := webEvidenceState{}

	for iter := 0; iter < cfg.MaxToolIterations; iter++ {
		// graph 기반 RenderForLLM 사용
		e.mu.RLock()
		sysBlocks := JoinSystemBlocks(systemFn(), workspaceSystemBlocks(deps.Workspace))
		systemTokens := estimateSystemTokens(sysBlocks)
		contextWin := activeModelContextWindow(appState)
		if contextWin <= 0 {
			contextWin = cfg.ContextWindowFallback
		}
		messages := ctxpkg.RenderForLLM(e.graph, e.est, systemTokens, contextWin, currentUserText)
		e.mu.RUnlock()

		toolSpecs := registry.ToolSpecs(tool.Context{
			Context:         ctx,
			State:           appState,
			WorkingDir:      deps.WorkingDir,
			Workspace:       deps.Workspace,
			ShellJobs:       deps.ShellJobs,
			ExecuteSubQuery: e.ExecuteSubQuery,
		})
		if cfg.DebugTools {
			fmt.Printf("[DEBUG] Sending %d tools to model\n", len(toolSpecs))
			for _, t := range toolSpecs {
				fmt.Printf(" - Tool: %s\n", t.Name)
			}
		}

		req := llm.Request{
			System:    sysBlocks,
			Messages:  messages,
			Tools:     toolSpecs,
			MaxTokens: cfg.MaxTokens,
		}

		// Token count and limit check
		estimatedTokens := 0
		if tokens, err := client.CountTokens(ctx, req); err == nil {
			estimatedTokens = tokens
			if contextWin > 0 {
				if appState != nil {
					appState.SetContextUsedPercent((tokens * 100) / contextWin)
				}
				// graph 기반에서는 초과 시 ForceConsolidate 후 재시도
				if tokens >= contextWin {
					e.mu.Lock()
					e.graph.ForceConsolidate("[이전 대화 내용이 컨텍스트 최적화를 위해 요약되었습니다.]")
					e.mu.Unlock()
					// 요약 후 재 render
					e.mu.RLock()
					messages = ctxpkg.RenderForLLM(e.graph, e.est, systemTokens, contextWin, currentUserText)
					e.mu.RUnlock()
					req.Messages = messages

					// 토큰 개수 재계산 (버그 수정)
					if newTokens, err := client.CountTokens(ctx, req); err == nil {
						tokens = newTokens
					}
				}
				remaining := contextWin - tokens
				if remaining <= 0 {
					// ForceConsolidate 후에도 예산이 없으면 최소값 보장
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

		// Streaming start
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
		bufferAssistantText := shouldBufferAssistantText(webPolicy, webState)

	streamLoop:
		for evt := range stream {
			switch evt.Kind {
			case llm.EventThinkingDelta:
				if evt.Delta != "" {
					thinkingOpen = true
					out <- UIEvent{Kind: UIEventThinkingDelta, Delta: evt.Delta}
				}
			case llm.EventTextDelta:
				if evt.Delta != "" {
					if thinkingOpen {
						thinkingOpen = false
						out <- UIEvent{Kind: UIEventThinkingDone}
					}
					e.budget.AddOutput(max(1, len([]rune(evt.Delta))/4))
					assistantText.WriteString(evt.Delta)
					if bufferAssistantText {
						webState.BufferedAssistantText += evt.Delta
					} else {
						out <- UIEvent{Kind: UIEventAssistantDelta, Delta: evt.Delta}
					}
				}
			case llm.EventToolUseStart:
				if evt.ToolUse != nil {
					if thinkingOpen {
						thinkingOpen = false
						out <- UIEvent{Kind: UIEventThinkingDone}
					}
					safeToolUse := sanitizeToolUseForDisplay(*evt.ToolUse)
					e.emitTrace(turnIndex, "llm.tool_use", client.Provider(), "", evt.ToolUse.ID, map[string]any{
						"name":  evt.ToolUse.Name,
						"input": json.RawMessage(safeToolUse.Input),
					})
					toolCalls = append(toolCalls, *evt.ToolUse)
					out <- UIEvent{
						Kind:    UIEventToolUse,
						ToolUse: &safeToolUse,
						TaskID:  strings.TrimSpace(evt.ToolUse.ID),
					}
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
		}
		if streamErr != nil {
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
		recordAssistant := true
		recordedAssistantText := assistantTextValue
		if bufferAssistantText {
			if len(toolCalls) == 0 {
				recordAssistant = false
			} else {
				recordedAssistantText = ""
			}
		}

		// Record assistant message (legacy messages + graph)
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

		// Termination condition: No tool calls and model declared end
		if len(toolCalls) == 0 {
			// 첫 번째 반복에서 도구 없이 텍스트만 반환한 경우 강제 재시도 (최대 1회)
			if iter == 0 && strings.TrimSpace(assistantTextValue) != "" && lastToolCalls == 0 {
				forceMsg := "CRITICAL: You responded with text only instead of using tools. This is unacceptable. You MUST immediately use bash, powershell, server_copy, or other tools to perform the task. Do NOT provide instructions or explanations — execute NOW using tool calls."
				e.mu.Lock()
				e.messages = append(e.messages, newUserTextMessage(forceMsg))
				e.graph.AppendUser(forceMsg)
				e.mu.Unlock()
				continue
			}
			if shouldForceWebEvidenceContinuation(webPolicy, webState) {
				webState.ForcedRetries++
				followUp := buildWebEvidenceFollowUpPrompt(webPolicy, webState)
				e.mu.Lock()
				e.messages = append(e.messages, newUserTextMessage(followUp))
				e.graph.AppendUser(followUp)
				e.mu.Unlock()
				out <- UIEvent{Kind: UIEventNotice, Notice: "continuing turn for web verification"}
				continue
			}
			if shouldBufferAssistantText(webPolicy, webState) {
				failure := buildWebEvidenceFailureMessage()
				e.mu.Lock()
				e.messages = append(e.messages, newAssistantMessage(failure, nil))
				e.graph.AppendAssistant(failure)
				e.mu.Unlock()
				out <- UIEvent{Kind: UIEventAssistantDelta, Delta: failure}
				e.emitTrace(turnIndex, "assistant.final", client.Provider(), "", "", map[string]any{
					"text":        failure,
					"chars":       len(failure),
					"stop_reason": string(stopReason),
				})
				out <- UIEvent{Kind: UIEventDone, StopReason: stopReason}
				e.callStopHooks(ctx, stopHooks, stopReason, len(toolCalls))
				return
			}
			e.emitTrace(turnIndex, "assistant.final", client.Provider(), "", "", map[string]any{
				"text":        assistantTextValue,
				"chars":       len(assistantTextValue),
				"stop_reason": string(stopReason),
			})
			out <- UIEvent{Kind: UIEventDone, StopReason: stopReason}
			e.callStopHooks(ctx, stopHooks, stopReason, len(toolCalls))
			return
		}

		// Execute tools and collect results (Parallelized)
		results := tools.RunToolsWithDispatcher(ctx, toolCalls, registry, e.hookRegistry, hookDispatcher, func(tCtx context.Context, call llm.ToolUseStart) (string, bool) {
			out <- UIEvent{Kind: UIEventNotice, Notice: fmt.Sprintf("executing tool: %s", call.Name)}
			result, isErr := e.invokeTool(tCtx, out, call, registry, appState, deps, false, turnIndex)
			return result, isErr
		})

		for _, res := range results {
			webState.ObserveToolCall(res.Call.Name, res.Output, res.IsError)
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
				out <- UIEvent{Kind: UIEventNotice, Notice: "history snipped for optimization"}
			}
		}
	}

	// Max iterations reached
	turnStop = lastStop
	turnError = true
	e.emitTrace(turnIndex, "error", client.Provider(), "", "", map[string]any{
		"stage": "engine.max_tool_iterations",
		"error": fmt.Sprintf("max tool iterations reached (%d)", cfg.MaxToolIterations),
	})
	out <- UIEvent{
		Kind:       UIEventError,
		Err:        fmt.Errorf("max tool iterations reached (%d)", cfg.MaxToolIterations),
		StopReason: lastStop,
	}
	e.callStopHooks(ctx, stopHooks, lastStop, lastToolCalls)
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

	toolCtx := tool.Context{
		Context:         ctx,
		State:           appState,
		WorkingDir:      deps.WorkingDir,
		Workspace:       deps.Workspace,
		ShellJobs:       deps.ShellJobs,
		ExecuteSubQuery: e.ExecuteSubQuery,
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
		e.emitTrace(turnIndex, "error", "", "", call.ID, map[string]any{
			"stage": "tool.permission",
			"error": fmt.Sprintf("permission denied for %s", call.Name),
		})
		return fmt.Sprintf("permission denied for %s", call.Name), true
	}

	startedAt := time.Now()
	e.emitTrace(turnIndex, "tool.call.start", "", "", call.ID, map[string]any{
		"tool":  call.Name,
		"input": json.RawMessage(safeInput),
	})
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

	// Distiller를 통해 계층적 압축 (distiller가 nil이면 기존 방식 유지)
	e.mu.RLock()
	dist := e.distiller
	gr := e.graph
	e.mu.RUnlock()

	if dist != nil && gr != nil {
		seq := gr.Len() + 1
		dr := dist.Distill(seq, call.Name, call.ID, text, isError)
		e.mu.Lock()
		gr.AppendToolResult(call.ID, call.Name, dr.InlineText, dr.Projection, isError, dr.ArtifactID, dr.ArtifactPath)
		e.mu.Unlock()
		// invokeTool은 legacy messages용 raw text를 반환; graph에는 이미 추가됨
		if dr.Projection != ctxpkg.ProjectionFull && dr.ArtifactPath != "" && !strings.Contains(dr.InlineText, dr.ArtifactPath) {
			return fmt.Sprintf("%s\n\n[전체 출력: %s]", dr.InlineText, dr.ArtifactPath), isError
		}
		return dr.InlineText, isError
	}

	// fallback: 기존 maxChars 절단
	text = ctxpkg.NormalizeToolResultForContext(call.Name, text)
	maxChars := toolImpl.MaxResultSizeChars()
	if maxChars > 0 && len(text) > maxChars {
		truncated := text[:maxChars]
		text = fmt.Sprintf("%s\n\n... [%d characters truncated] ...", truncated, len(text)-maxChars)
	}
	return text, isError
}

// performSnip 은 더 이상 사용되지 않으며 graph.ForceConsolidate()로 대체됩니다.
// legacy 코드와의 호환을 위해 남겨둡니다.
func (e *Engine) performSnip() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.graph.ForceConsolidate("[시스템: 이전 대화가 컨텍스트 최적화를 위해 정리되었습니다.]")
	// legacy messages도 동기화
	if len(e.messages) > 30 {
		keep := 15
		snippedCount := len(e.messages) - keep
		e.messages = e.messages[snippedCount:]
		summary := fmt.Sprintf("[시스템: 이전 대화 %d개가 컨텍스트 최적화를 위해 정리되었습니다.]", snippedCount)
		e.messages = append([]llm.Message{llm.TextMessage(llm.RoleUser, summary)}, e.messages...)
	}
}

func isShellTool(toolName string) bool {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "bash", "powershell":
		return true
	default:
		return false
	}
}

func extractOutputTaskID(output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return ""
	}
	if v, ok := payload["OutputTaskID"].(string); ok {
		return strings.TrimSpace(v)
	}
	if v, ok := payload["output_task_id"].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func isPlanModeWriteException(toolName string) bool {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "todowrite", "exitplanmode", "exit_plan_mode":
		return true
	default:
		return false
	}
}

func findPreApprovedRuleLegacy(appState *state.AppState, toolName string, input json.RawMessage) *types.PermissionRule {
	rules := permutils.LoadAllPermissionRulesFromDisk()
	if appState != nil {
		rules = append(rules, appState.SessionPermissionRules()...)
	}
	if len(rules) == 0 {
		return nil
	}

	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return nil
	}
	command := strings.TrimSpace(tool.ExtractStringInput(input, "command"))
	isShellTool := strings.EqualFold(toolName, "bash") || strings.EqualFold(toolName, "powershell")
	caseInsensitiveShell := strings.EqualFold(toolName, "powershell")

	for _, rule := range rules {
		if rule.RuleBehavior != types.BehaviorAllow {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(rule.RuleValue.ToolName), toolName) {
			continue
		}
		content := strings.TrimSpace(rule.RuleValue.RuleContent)
		if content == "" {
			matched := rule
			return &matched
		}
		if !isShellTool || command == "" {
			continue
		}
		parsed := permutils.ParsePermissionRule(content)
		if permutils.MatchesPermissionRule(parsed, command, caseInsensitiveShell) {
			matched := rule
			return &matched
		}
	}
	return nil
}

func (e *Engine) appendToolResult(toolUseID, toolName, text string, isError bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	safeText := sanitizeTextWithSecrets(text, nil)
	e.messages = append(e.messages, newToolResultMessage(toolUseID, toolName, safeText, isError))
	// graph는 invokeTool 내 distiller 경로에서 이미 AppendToolResult 됨.
	// distiller가 nil인 fallback 경우에만 여기서 추가
	if e.distiller == nil {
		e.graph.AppendToolResult(toolUseID, toolName, safeText, ctxpkg.ProjectionFull, isError, "", "")
	}
}

func (e *Engine) callStopHooks(ctx context.Context, stopHooks []StopHook, stop llm.StopReason, toolCalls int) {
	// Stop 이벤트 훅 비동기 발화
	if d := e.hookDispatcher; d != nil && !d.IsEmpty() {
		go d.Dispatch(ctx, types.HookEventStop, hooks.HookInput{})
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

func activeModelContextWindow(appState *state.AppState) int {
	if appState == nil {
		return 0
	}
	return appState.ActiveModelContext()
}

func buildPlannedStepToolCallInput(toolName, prompt, workingDir string) (json.RawMessage, error) {
	body := map[string]any{
		"command": prompt,
	}
	if strings.TrimSpace(workingDir) != "" {
		body["workdir"] = workingDir
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func parsePlanExecutionReady(toolName, result string, isErr bool) []PlannedStep {
	if isErr || !isExitPlanModeToolName(toolName) {
		return nil
	}
	var payload struct {
		AllowedPrompts []struct {
			Tool   string `json:"tool"`
			Prompt string `json:"prompt"`
		} `json:"allowed_prompts"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		return nil
	}
	out := make([]PlannedStep, 0, len(payload.AllowedPrompts))
	for _, item := range payload.AllowedPrompts {
		toolName := strings.ToLower(strings.TrimSpace(item.Tool))
		prompt := strings.TrimSpace(item.Prompt)
		if (toolName == "bash" || toolName == "powershell") && prompt != "" {
			out = append(out, PlannedStep{Tool: toolName, Prompt: prompt})
		}
	}
	return out
}

func isExitPlanModeToolName(toolName string) bool {
	name := strings.TrimSpace(toolName)
	return strings.EqualFold(name, constants.ExitPlanModeToolName) ||
		strings.EqualFold(name, constants.LegacyExitPlanModeToolName)
}

func (e *Engine) emitTrace(turnIndex int, traceType, provider, model, callID string, data any) {
	if turnIndex <= 0 {
		return
	}
	e.mu.Lock()
	deps := e.deps
	if !deps.AIDebug.Enabled || deps.AIDebug.Emitter == nil {
		e.mu.Unlock()
		return
	}
	e.debugSeq++

	// 큰 데이터 절삭 처리
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
	emitter.Emit(record)
}

func truncateLargeData(data any) any {
	const maxLen = 8192
	switch v := data.(type) {
	case string:
		if len(v) > maxLen {
			return v[:maxLen/2] + fmt.Sprintf("\n...[truncated %d bytes]...\n", len(v)-maxLen) + v[len(v)-maxLen/2:]
		}
	case json.RawMessage:
		s := string(v)
		if len(s) > maxLen {
			s = s[:maxLen/2] + fmt.Sprintf("\n...[truncated %d bytes]...\n", len(s)-maxLen) + s[len(s)-maxLen/2:]
			return json.RawMessage([]byte(s))
		}
		return v
	case []byte:
		s := string(v)
		if len(s) > maxLen {
			s = s[:maxLen/2] + fmt.Sprintf("\n...[truncated %d bytes]...\n", len(s)-maxLen) + s[len(s)-maxLen/2:]
			return []byte(s)
		}
		return v
	case map[string]any:
		newMap := make(map[string]any)
		for k, val := range v {
			newMap[k] = truncateLargeData(val)
		}
		return newMap
	case []any:
		newSlice := make([]any, len(v))
		for i, val := range v {
			newSlice[i] = truncateLargeData(val)
		}
		return newSlice
	}
	return data
}
