// Package query
// File: tool_invoker.go
// Description: PreToolUse/PostToolUse integration wrapper around tool execution.
package query

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/yourorg/infractl/internal/agent/planmode"
	todoagent "github.com/yourorg/infractl/internal/agent/todo"
	"github.com/yourorg/infractl/internal/hooks"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/tools"
)

// BlockedCallback is called when a tool is blocked before execution.
type BlockedCallback func(tc llm.ToolCall, reason string)

var leadingPrivilegeWrapperRe = regexp.MustCompile(`(?i)^\s*(sudo|su|runuser)\b`)

// maxRepeatToolCalls is the number of consecutive identical tool calls before a warning is injected.
const maxRepeatToolCalls = 3

// ToolInvoker wraps the base runner with gates + hook execution.
type ToolInvoker struct {
	hookRunner      *hooks.Runner
	base            ToolRunner
	session         hooks.HookSession
	registry        *tools.Registry
	planState       *planmode.State
	planFilter      *planmode.ReadOnlyFilter
	todoEnforcer    *todoagent.Enforcer
	blockedCallback BlockedCallback
	isElevatedMode  func() bool
	isYoloMode      func() bool
	approvalRequest func(context.Context, string) (bool, error)

	lastToolKey string // "<name>|<args>" of the most recent call
	lastResult  string // Output of the most recent call
	repeatCount int    // consecutive repeat counter
	retryCount  int    // one-shot post-hook rewrite retry counter per tool call
}

// NewToolInvoker builds a ToolInvoker.
func NewToolInvoker(hookRunner *hooks.Runner, base ToolRunner) *ToolInvoker {
	return &ToolInvoker{hookRunner: hookRunner, base: base}
}

// SetPlanGate injects plan-mode state + readonly filter.
func (ti *ToolInvoker) SetPlanGate(state *planmode.State, filter *planmode.ReadOnlyFilter) {
	ti.planState = state
	ti.planFilter = filter
}

// SetTodoEnforcer injects TodoWrite enforcer.
func (ti *ToolInvoker) SetTodoEnforcer(e *todoagent.Enforcer) {
	ti.todoEnforcer = e
}

// SetBlockedCallback registers a callback for blocked tools.
func (ti *ToolInvoker) SetBlockedCallback(cb BlockedCallback) {
	ti.blockedCallback = cb
}

// SetSession sets hook session metadata.
func (ti *ToolInvoker) SetSession(s hooks.HookSession) {
	ti.session = s
}

// SetRegistry injects tool registry for readonly classification.
func (ti *ToolInvoker) SetRegistry(reg *tools.Registry) {
	ti.registry = reg
}

// SetIsElevatedMode injects elevated-mode status provider.
func (ti *ToolInvoker) SetIsElevatedMode(fn func() bool) {
	ti.isElevatedMode = fn
}

// SetIsYoloMode injects YOLO-mode status provider for auto-approval.
func (ti *ToolInvoker) SetIsYoloMode(fn func() bool) {
	ti.isYoloMode = fn
}

// SetApprovalRequester injects an approval prompt callback.
func (ti *ToolInvoker) SetApprovalRequester(fn func(context.Context, string) (bool, error)) {
	ti.approvalRequest = fn
}

// Invoke applies gates -> pre-hook -> base execution -> post-hook.
func (ti *ToolInvoker) Invoke(ctx context.Context, tc llm.ToolCall) (string, bool, string) {
	ti.retryCount = 0

	// Tight-loop guard: warn the LLM when the same tool+args are called consecutively.
	callKey := tc.Function.Name + "|" + tc.Function.Arguments

	// Permission denied loop guard.
	if strings.Contains(ti.lastResult, "Permission denied") {
		args := parseArgsForHook(tc.Function.Arguments)
		isEscalated := isPrivilegeRelated(tc.Function.Name, args)
		if !isEscalated {
			ti.repeatCount++
			slog.Warn("permission-denied loop detected", "tool", tc.Function.Name, "count", ti.repeatCount)
			msg := "Error: previous command failed with Permission denied. Retry only with explicit escalation fields such as become_method/become_user."
			return msg, true, ""
		}
	}

	if callKey == ti.lastToolKey {
		ti.repeatCount++
		if ti.repeatCount >= maxRepeatToolCalls {
			slog.Warn("tight-loop detected: consecutive identical tool calls",
				"tool", tc.Function.Name, "count", ti.repeatCount)
			return fmt.Sprintf("Repeated identical tool call (%s) %d times. Use a different approach.", tc.Function.Name, ti.repeatCount), false, ""
		}
	} else {
		ti.lastToolKey = callKey
		ti.repeatCount = 1
	}

	// Plan mode hard gate.
	if ti.planState != nil && ti.planState.IsActive() && ti.planFilter != nil {
		args := parseArgsForHook(tc.Function.Arguments)
		md := ComputeMetadata(tc.Function.Name, args)
		if allowed, reason := ti.planFilter.Allow(tc.Function.Name, md); !allowed {
			id := ti.planState.Queue().Add(tc.Function.Name, args)
			slog.Info("plan mode blocked tool call",
				"tool", tc.Function.Name,
				"pending_id", id,
				"reason", reason,
			)
			msg := fmt.Sprintf("Error: %s (pending_id=%s)", reason, id)
			if ti.blockedCallback != nil {
				ti.blockedCallback(tc, msg)
			}
			return msg, true, ""
		}
	}

	// TodoWrite enforcer.
	if ti.todoEnforcer != nil && (ti.planState == nil || !ti.planState.IsActive()) {
		args := parseArgsForHook(tc.Function.Arguments)
		md := ComputeMetadata(tc.Function.Name, args)
		isReadOnly := md.ReadOnly
		if !isReadOnly && ti.registry != nil {
			if tool, ok := ti.registry.Get(tc.Function.Name); ok && tool.IsReadOnly() {
				isReadOnly = true
			}
		}
		if ok, reason := ti.todoEnforcer.Enforce(tc.Function.Name, isReadOnly); !ok {
			slog.Info("todo enforcer blocked tool call", "tool", tc.Function.Name, "reason", reason)
			msg := fmt.Sprintf("Error: %s", reason)
			if ti.blockedCallback != nil {
				ti.blockedCallback(tc, msg)
			}
			return msg, true, ""
		}
	}

	// PreToolUse hook.
	if ti.hookRunner != nil {
		args := parseArgsForHook(tc.Function.Arguments)
		hookIn := hooks.HookInput{
			Tool:     tc.Function.Name,
			Input:    args,
			Session:  ti.session,
			Metadata: ComputeMetadata(tc.Function.Name, args),
		}
		hookOut := ti.hookRunner.RunPreToolUse(ctx, hookIn)
		if hookOut.IsDeny() {
			if !ti.shouldBypassHookDeny(tc.Function.Name, args, hookOut) {
				msg := hookOut.SystemMessage
				if msg == "" {
					msg = hookOut.Reason
				}
				if msg == "" {
					msg = "denied by hook"
				}
				slog.Info("tool denied by PreToolUse hook",
					"tool", tc.Function.Name,
					"decision", string(hookOut.Decision),
					"reason", hookOut.Reason,
				)
				return fmt.Sprintf("Error: tool denied by PreToolUse hook: %s", msg), true, ""
			}
			slog.Info("hook deny bypassed in elevated mode",
				"tool", tc.Function.Name,
				"decision", string(hookOut.Decision),
				"runtime_error", hookOut.RuntimeError,
				"reason", hookOut.Reason,
			)
		}
		if len(hookOut.NewInput) > 0 {
			if newArgs, err := json.Marshal(hookOut.NewInput); err == nil {
				tc.Function.Arguments = string(newArgs)
				slog.Debug("PreToolUse hook modified tool input", "tool", tc.Function.Name)
			}
		}
	}

	// Base execution.
	output, isError, metadataJSON := ti.base(ctx, tc)

	// Save last result for loop guards.
	ti.lastResult = output

	// PostToolUse hook.
	if ti.hookRunner != nil {
		args := parseArgsForHook(tc.Function.Arguments)
		hookIn := hooks.HookInput{
			Tool:     tc.Function.Name,
			Input:    args,
			Session:  ti.session,
			Metadata: ComputeMetadata(tc.Function.Name, args),
			Output: map[string]any{
				"output":   output,
				"is_error": isError,
				"stderr":   output,
			},
		}
		hookOut := ti.hookRunner.RunPostToolUse(ctx, hookIn)
		if hookOut.Decision == hooks.DecisionAsk && len(hookOut.NewInput) > 0 && ti.retryCount < 1 {
			approved, reqErr := ti.requestApproval(ctx, hookOut.SystemMessage)
			if reqErr != nil {
				slog.Warn("post-hook approval request failed", "tool", tc.Function.Name, "err", reqErr)
			}
			if approved {
				newArgs, err := json.Marshal(hookOut.NewInput)
				if err != nil {
					slog.Warn("failed to marshal post-hook retry args", "tool", tc.Function.Name, "err", err)
				} else {
					tc.Function.Arguments = string(newArgs)
					ti.retryCount++
					output, isError, metadataJSON = ti.base(ctx, tc)
					ti.lastResult = output
				}
			}
		}
	}

	return output, isError, metadataJSON
}

// AsToolRunner exposes Invoke as ToolRunner.
func (ti *ToolInvoker) AsToolRunner() ToolRunner {
	return ti.Invoke
}

func (ti *ToolInvoker) shouldBypassHookDeny(toolName string, args map[string]any, out hooks.HookOutput) bool {
	if !ti.isElevatedEnabled() {
		return false
	}
	if !isPrivilegeRelated(toolName, args) {
		return false
	}
	if out.Decision == hooks.DecisionAsk {
		return true
	}
	return out.Decision == hooks.DecisionDeny && out.RuntimeError
}

func (ti *ToolInvoker) isElevatedEnabled() bool {
	return ti.isElevatedMode != nil && ti.isElevatedMode()
}

func (ti *ToolInvoker) requestApproval(ctx context.Context, message string) (bool, error) {
	if ti.isYoloMode != nil && ti.isYoloMode() {
		return true, nil
	}
	if ti.approvalRequest == nil {
		return false, nil
	}
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = "Retry with sudo?"
	}
	approved, err := ti.approvalRequest(ctx, msg)
	if err != nil {
		return false, fmt.Errorf("request permission retry approval: %w", err)
	}
	return approved, nil
}

func isPrivilegeRelated(toolName string, args map[string]any) bool {
	if !strings.EqualFold(strings.TrimSpace(toolName), "shell_exec") {
		return false
	}
	if method, ok := args["become_method"].(string); ok && strings.TrimSpace(method) != "" {
		return true
	}
	if raw, ok := args["command"].(string); ok {
		cmd := strings.TrimSpace(raw)
		if leadingPrivilegeWrapperRe.MatchString(cmd) {
			return true
		}
		lower := strings.ToLower(cmd)
		return strings.Contains(lower, "| sudo ") ||
			strings.Contains(lower, "&& sudo ") ||
			strings.Contains(lower, "; sudo ") ||
			strings.Contains(lower, "| su ") ||
			strings.Contains(lower, "&& su ") ||
			strings.Contains(lower, "; su ") ||
			strings.Contains(lower, "| runuser ") ||
			strings.Contains(lower, "&& runuser ") ||
			strings.Contains(lower, "; runuser ")
	}
	return false
}

// parseArgsForHook parses raw tool-call JSON arguments for hook metadata.
func parseArgsForHook(arguments string) map[string]any {
	var m map[string]any
	if err := json.Unmarshal([]byte(arguments), &m); err != nil {
		return map[string]any{}
	}
	return m
}

// ParseArgsForCallback is exported for callback callers.
func ParseArgsForCallback(arguments string) map[string]any {
	return parseArgsForHook(arguments)
}
