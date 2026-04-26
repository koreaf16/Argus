// Package privilege
// File: executor_wrapper.go
// Description: Zero-Trust Auto-Elevation Executor Wrapper
// Responsibility: Transparently escalate privileges (sudo/su) when permission errors are detected

package privilege

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

// ElevatingExecutor wraps a base executor and adds automatic privilege escalation.
type ElevatingExecutor struct {
	inner   executor.Executor
	cache   *Cache
	handler PromptHandler
}

// NewElevatingExecutor creates a new wrapper that auto-elevates on permission failure.
func NewElevatingExecutor(inner executor.Executor, cache *Cache, handler PromptHandler) *ElevatingExecutor {
	return &ElevatingExecutor{
		inner:   inner,
		cache:   cache,
		handler: handler,
	}
}

func (e *ElevatingExecutor) Execute(ctx context.Context, command string) (executor.ExecResult, error) {
	// 1. Try normally first
	result, err := e.inner.Execute(ctx, command)
	
	// 2. If it succeeded or it's not a permission error, return immediately
	if err == nil && result.ExitCode == 0 {
		return result, nil
	}

	if !executor.IsPermissionFailure(result) {
		return result, err
	}

	// 3. Auto-Elevation Triggered
	// We only auto-elevate on Linux/Unix-like targets.
	if executor.CommandPlatform(e.inner) == executor.PlatformWindows {
		return result, err
	}

	// For simplicity, we try sudo root by default for auto-elevation.
	// In the future, we could infer the best user based on previous context.
	elevated, ok, elevErr := e.executeWithBecome(ctx, command, "sudo", "root")
	if !ok || elevErr != nil {
		// If elevation fails or is not possible, return the original permission error.
		return result, err
	}

	// Mark that this result was obtained via auto-elevation for transparency.
	if elevated.ExitCode == 0 {
		elevated.Stdout = "[Auto-Elevated via sudo]\n" + elevated.Stdout
	}
	
	return elevated, nil
}

func (e *ElevatingExecutor) Target() string { return e.inner.Target() }
func (e *ElevatingExecutor) Host() string   { return e.inner.Host() }

func (e *ElevatingExecutor) ExecuteStream(ctx context.Context, command string, onLine func(string)) (executor.ExecSession, error) {
	se, ok := e.inner.(executor.StreamExecutor)
	if !ok {
		return nil, fmt.Errorf("underlying executor does not support streaming")
	}

	// For streaming, we don't auto-elevate AFTER failure easily because the stream is consumed.
	// However, we can pre-check or just let the caller handle it.
	// For now, we delegate to inner.
	return se.ExecuteStream(ctx, command, onLine)
}

func (e *ElevatingExecutor) ExecuteInteractive(ctx context.Context, spec executor.InteractiveSpec, onChunk func(string)) (executor.ExecSession, error) {
	ie, ok := e.inner.(executor.InteractiveExecutor)
	if !ok {
		return nil, fmt.Errorf("underlying executor does not support interactive execution")
	}
	return ie.ExecuteInteractive(ctx, spec, onChunk)
}

// executeWithBecome is a simplified version of the tool-level logic,
// adapted to work at the executor level.
func (e *ElevatingExecutor) executeWithBecome(ctx context.Context, command, rawMethod, user string) (executor.ExecResult, bool, error) {
	method, ok := ParseMethod(rawMethod)
	if !ok || method == MethodNone {
		return executor.ExecResult{}, false, nil
	}

	plan, err := BuildPlan(method, user, command)
	if err != nil {
		return executor.ExecResult{}, true, err
	}

	// Try non-interactive first if sudo is configured for NOPASSWD
	if method == MethodSudo {
		res, _ := e.inner.Execute(ctx, plan.NonInteractiveRun)
		if res.ExitCode == 0 {
			return res, true, nil
		}
	}

	// If we need interactive elevation (password), we require an InteractiveExecutor.
	ie, ok := e.inner.(executor.InteractiveExecutor)
	if !ok {
		return executor.ExecResult{}, false, nil
	}

	target := e.inner.Target()
	if strings.TrimSpace(target) == "" {
		target = "localhost"
	}
	normUser := NormalizeUser(user)

	pw, _ := e.cache.Get(target, method, normUser)
	if strings.TrimSpace(pw) == "" {
		if e.handler == nil {
			return executor.ExecResult{}, false, nil
		}
		resp, err := e.handler.RequestPassword(ctx, PromptRequest{Target: target, Method: method, User: normUser})
		if err != nil || resp.Abort {
			return executor.ExecResult{}, false, err
		}
		pw = resp.Password
	}

	// Execute interactively and inject password if prompted
	var chunks []string
	passwordInjected := false
	
	// Note: Stdin injection logic at executor level might need a bridge.
	// For now, we assume the underlying SSH/Local executor handles PTY interaction.
	
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var session executor.ExecSession
	session, err = ie.ExecuteInteractive(runCtx, executor.InteractiveSpec{Command: plan.InteractiveRun, RequirePTY: plan.RequirePTY}, func(chunk string) {
		chunks = append(chunks, chunk)
		if passwordInjected {
			return
		}
		combined := strings.Join(chunks, "")
		if CountPromptMatches(method, combined, plan.PromptToken) > 0 {
			passwordInjected = true
			// We need to inject stdin.
			if session != nil {
				_ = session.InjectStdin(pw)
			}
		}
	})
	if err != nil {
		return executor.ExecResult{}, true, err
	}

	run, waitErr := session.Wait()
	if waitErr != nil {
		return executor.ExecResult{}, true, waitErr
	}

	combined := strings.Join(chunks, "")
	run.Stdout = SanitizeOutput(method, combined, plan.PromptToken)
	
	if IsAuthFailure(combined, CountPromptMatches(method, combined, plan.PromptToken)) {
		e.cache.Delete(target, method, normUser)
	} else if run.ExitCode == 0 {
		e.cache.Set(target, method, normUser, pw)
	}

	executor.CategorizeResult(&run)
	return run, true, nil
}
