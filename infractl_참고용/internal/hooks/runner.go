// Package hooks
// File: runner.go
// Description: Hook execution orchestrator for PreToolUse/PostToolUse.
package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/yourorg/infractl/internal/llm"
)

// Runner executes matched hooks for each event.
type Runner struct {
	llmRegistry *llm.Registry
	http        *httpBackend
	agentRunner AgentRunner
	onceMu      sync.Mutex
	onceSeen    map[string]struct{}
}

// NewRunner builds a hook runner.
func NewRunner(llmRegistry *llm.Registry) *Runner {
	return &Runner{
		llmRegistry: llmRegistry,
		http:        newHTTPBackend(),
		onceSeen:    make(map[string]struct{}),
	}
}

// SetAgentRunner injects the agent backend runner.
func (r *Runner) SetAgentRunner(runner AgentRunner) {
	r.agentRunner = runner
}

// RunPostToolUse runs post hooks in best-effort mode.
func (r *Runner) RunPostToolUse(ctx context.Context, input HookInput) HookOutput {
	input.Event = HookEventPostToolUse
	return r.runEvent(ctx, HookEventPostToolUse, input, false)
}

// RunPreToolUse runs pre hooks in fail-closed mode.
func (r *Runner) RunPreToolUse(ctx context.Context, input HookInput) HookOutput {
	input.Event = HookEventPreToolUse
	return r.runEvent(ctx, HookEventPreToolUse, input, true)
}

func (r *Runner) runEvent(ctx context.Context, event HookEvent, input HookInput, failClosed bool) HookOutput {
	cfg := GetSnapshot()
	matchers := cfg.MatchersFor(event)
	if len(matchers) == 0 {
		return HookOutput{Decision: DecisionAllow}
	}

	defs, matched := MatchInput(matchers, input)
	if !matched {
		return HookOutput{Decision: DecisionAllow}
	}

	merged := HookOutput{Decision: DecisionAllow}
	for _, def := range defs {
		if def.Once && r.alreadyRan(event, def) {
			continue
		}
		if def.Async {
			r.markRan(event, def)
			go r.runAsync(ctx, event, input, def)
			continue
		}

		r.markRan(event, def)
		out, err := r.runSingle(ctx, def, input)
		if err != nil {
			slog.Warn("hook execution failed",
				"event", event,
				"tool", input.Tool,
				"backend", def.Type,
				"err", err,
			)
			if failClosed {
				return HookOutput{
					Decision:      DecisionDeny,
					Reason:        fmt.Sprintf("hook error: %s", err),
					SystemMessage: fmt.Sprintf("Hook execution failed and blocked tool call: %s", err),
					RuntimeError:  true,
				}
			}
			continue
		}

		slog.Debug("hook executed",
			"event", event,
			"tool", input.Tool,
			"backend", def.Type,
			"decision", string(out.Decision),
		)

		if out.IsDeny() {
			return out
		}
		if len(out.NewInput) > 0 {
			merged.NewInput = out.NewInput
		}
		if out.SystemMessage != "" {
			merged.SystemMessage = out.SystemMessage
		}
	}
	return merged
}

func (r *Runner) runAsync(ctx context.Context, event HookEvent, input HookInput, def HookDef) {
	out, err := r.runSingle(ctx, def, input)
	if err != nil {
		slog.Warn("hook execution failed",
			"event", event,
			"tool", input.Tool,
			"backend", def.Type,
			"err", err,
		)
		return
	}
	if out.IsDeny() {
		slog.Debug("async hook denied tool call",
			"event", event,
			"tool", input.Tool,
			"backend", def.Type,
			"decision", string(out.Decision),
		)
	}
}

func (r *Runner) markRan(event HookEvent, def HookDef) {
	key := hookOnceKey(event, def)
	r.onceMu.Lock()
	r.onceSeen[key] = struct{}{}
	r.onceMu.Unlock()
}

func (r *Runner) alreadyRan(event HookEvent, def HookDef) bool {
	key := hookOnceKey(event, def)
	r.onceMu.Lock()
	defer r.onceMu.Unlock()
	_, ok := r.onceSeen[key]
	return ok
}

func hookOnceKey(event HookEvent, def HookDef) string {
	b, _ := json.Marshal(def)
	return string(event) + "|" + string(b)
}

func (r *Runner) runSingle(ctx context.Context, def HookDef, input HookInput) (HookOutput, error) {
	timeout := HookTimeout(def)
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	b, err := r.selectBackend(def)
	if err != nil {
		return HookOutput{}, err
	}
	return b.Run(tctx, def, input)
}

func (r *Runner) selectBackend(def HookDef) (Backend, error) {
	switch def.Type {
	case BackendCommand:
		return &commandBackend{}, nil
	case BackendPrompt:
		if r.llmRegistry == nil {
			return nil, errors.New("prompt backend: llm registry not configured")
		}
		return &promptBackend{registry: r.llmRegistry}, nil
	case BackendHTTP:
		return r.http, nil
	case BackendAgent:
		return &agentBackend{runner: r.agentRunner}, nil
	case BackendBuiltin:
		return &builtinBackend{}, nil
	default:
		return nil, fmt.Errorf("unknown backend type: %q", def.Type)
	}
}
