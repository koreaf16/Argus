package tools

import (
	"context"
	"fmt"
	"sync"

	"github.com/koreaf16/argus/internal/hooks"
	"github.com/koreaf16/argus/internal/services/llm"
	tool "github.com/koreaf16/argus/internal/tools"
)

type StreamingToolExecutor struct {
	ctx          context.Context
	registry     *tool.Registry
	hookRegistry *HookRegistry
	dispatcher   *hooks.HookDispatcher
	executor     func(context.Context, llm.ToolUseStart) (string, bool)
	max          int

	mu              sync.Mutex
	cond            *sync.Cond
	calls           []*trackedStreamingTool
	executing       int
	unsafeExecuting bool
	completed       int
	cancelled       bool
}

type trackedStreamingTool struct {
	call      llm.ToolUseStart
	safe      bool
	started   bool
	completed bool
	result    ToolResult
	cancel    context.CancelFunc
}

func NewStreamingToolExecutor(
	ctx context.Context,
	registry *tool.Registry,
	hookRegistry *HookRegistry,
	dispatcher *hooks.HookDispatcher,
	executor func(context.Context, llm.ToolUseStart) (string, bool),
) *StreamingToolExecutor {
	if ctx == nil {
		ctx = context.Background()
	}
	e := &StreamingToolExecutor{
		ctx:          ctx,
		registry:     registry,
		hookRegistry: hookRegistry,
		dispatcher:   dispatcher,
		executor:     executor,
		max:          MaxToolConcurrency(),
	}
	e.cond = sync.NewCond(&e.mu)
	return e
}

func (e *StreamingToolExecutor) Add(call llm.ToolUseStart) {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.calls = append(e.calls, &trackedStreamingTool{
		call: call,
		safe: isCallConcurrencySafe(call, e.registry),
	})
	e.scheduleLocked()
	e.mu.Unlock()
}

func (e *StreamingToolExecutor) CloseAndWait() []ToolResult {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	for e.completed < len(e.calls) {
		e.scheduleLocked()
		e.cond.Wait()
	}
	results := make([]ToolResult, len(e.calls))
	for i, tr := range e.calls {
		results[i] = tr.result
	}
	e.cancelled = true
	e.mu.Unlock()
	return results
}

func (e *StreamingToolExecutor) scheduleLocked() {
	if e.max <= 0 {
		e.max = 1
	}
	if e.cancelled {
		return
	}
	for _, tr := range e.calls {
		if tr.started || tr.completed {
			continue
		}
		if tr.safe {
			if e.unsafeExecuting || e.executing >= e.max {
				return
			}
			e.startLocked(tr)
			continue
		}
		if e.executing == 0 {
			e.startLocked(tr)
		}
		return
	}
}

func (e *StreamingToolExecutor) startLocked(tr *trackedStreamingTool) {
	tr.started = true
	e.executing++
	if !tr.safe {
		e.unsafeExecuting = true
	}
	go e.run(tr)
}

func (e *StreamingToolExecutor) run(tr *trackedStreamingTool) {
	call := tr.call
	// 각 도구 호출마다 독립적인 컨텍스트 부여 (쉘 도구 여부와 상관없이)
	toolCtx, cancel := context.WithCancel(e.ctx)
	tr.cancel = cancel
	defer cancel()

	result := e.runOne(toolCtx, call)

	e.mu.Lock()
	tr.result = result
	tr.completed = true
	e.completed++
	e.executing--
	if !tr.safe {
		e.unsafeExecuting = false
	}
	e.scheduleLocked()
	e.cond.Broadcast()
	e.mu.Unlock()
}

func (e *StreamingToolExecutor) runOne(ctx context.Context, call llm.ToolUseStart) ToolResult {
	if e.hookRegistry != nil {
		_ = e.hookRegistry.RunPreHooks(ctx, call)
	}
	if blocked, msg := runPreDispatch(ctx, e.dispatcher, call); blocked {
		return ToolResult{Call: call, Output: msg, IsError: true}
	}
	if e.executor == nil {
		return ToolResult{Call: call, Output: fmt.Sprintf("tool executor unavailable: %s", call.Name), IsError: true}
	}
	res, isErr := e.executor(ctx, call)
	if e.hookRegistry != nil {
		_ = e.hookRegistry.RunPostHooks(ctx, call, res, isErr)
	}
	runPostDispatch(ctx, e.dispatcher, call, res, isErr)
	return ToolResult{Call: call, Output: res, IsError: isErr}
}

func isShellLikeTool(name string) bool {
	switch tool.CanonicalName(name) {
	case "bash", "powershell":
		return true
	default:
		return false
	}
}
