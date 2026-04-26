// Package agent
// File: task_guard.go
// Description: 도구 실행 직전 TaskContext 위반을 판정하는 가드
// Responsibility: Forbidden 패턴 매칭(block), 서버/계정 불일치(warn)

package agent

import (
	"strings"

	"github.com/yourorg/infractl/internal/agent/taskctx"
)

// GuardResult holds the verdict for a tool call against the current TaskContext.
type GuardResult struct {
	Blocked bool
	Warned  bool
	Reason  string
}

// evaluateTaskGuard checks a pending tool call against the active TaskContext.
// It is called from executeSingleTool before actual tool execution.
//
// Block condition: tool args contain a shell command that matches any Forbidden pattern.
// Warn condition: tool's target differs from TaskContext.TargetServer (non-empty check).
func evaluateTaskGuard(
	gr *taskctx.Guardrails,
	tc *taskctx.TaskContext,
	toolName string,
	args map[string]any,
	target string,
) GuardResult {
	if gr == nil || tc == nil {
		return GuardResult{}
	}

	// Delegate full evaluation to Guardrails (Forbidden + server/account/dir mismatches).
	verdict := gr.Evaluate(tc, "", toolName, args, target)

	if verdict.Blocked {
		return GuardResult{Blocked: true, Reason: verdict.Reason}
	}
	if verdict.Warned {
		return GuardResult{Warned: true, Reason: verdict.Reason}
	}

	// Additional server mismatch warning when Guardrails did not already catch it
	// (non-empty target differs from TaskContext.TargetServer).
	if tc.TargetServer != "" && target != "" &&
		!strings.EqualFold(tc.TargetServer, target) {
		return GuardResult{
			Warned: true,
			Reason: "대상 서버 '" + target + "'이(가) 선언된 작업 서버 '" + tc.TargetServer + "'와 다릅니다",
		}
	}

	return GuardResult{}
}

// notifyGuardResult fires OnGuardViolation on the handler if it implements TaskEventSink.
func notifyGuardResult(handler EventHandler, toolName string, result GuardResult) {
	if s, ok := handler.(TaskEventSink); ok {
		s.OnGuardViolation(toolName, result.Reason, result.Blocked)
	}
}

// injectGuardWarnToArgs prepends a warning prefix to the tool's description arg (if any).
// Used when Warned=true to make the warning visible in TUI tool box.
func injectGuardWarnToArgs(args map[string]any, reason string) map[string]any {
	if desc, ok := args["description"].(string); ok {
		args["description"] = "[경고] " + reason + " — " + desc
	}
	return args
}
