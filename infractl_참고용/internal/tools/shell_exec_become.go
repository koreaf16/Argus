// Package tools
// File: shell_exec_become.go
// Description: sudo/su privilege execution path.
// Responsibility: execute commands through managed become_method/become_user flows.

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/privilege"
)

// executeWithBecome executes command through sudo or su.
//
// It first tries non-interactive paths, then falls back to PTY-based execution
// so sudo/su password prompts can be detected and answered through the managed
// privilege prompt handler.
func executeWithBecome(
	ctx context.Context,
	exec executor.Executor,
	command string,
	rawMethod string,
	user string,
	cache *privilege.Cache,
	prompter privilege.PromptHandler,
) (executor.ExecResult, bool, error) {
	method, ok := privilege.ParseMethod(rawMethod)
	if !ok || method == privilege.MethodNone {
		return executor.ExecResult{}, false, nil
	}
	if executor.CommandPlatform(exec) == executor.PlatformWindows {
		return executor.ExecResult{}, false, fmt.Errorf("privilege escalation is not supported on Windows targets")
	}

	plan, err := privilege.BuildPlan(method, user, command)
	if err != nil {
		return executor.ExecResult{}, true, err
	}

	if plan.UserCheckCommand != "" {
		check, checkErr := exec.Execute(ctx, plan.UserCheckCommand)
		if checkErr != nil {
			return executor.ExecResult{}, true, checkErr
		}
		if check.ExitCode != 0 {
			msg := fmt.Sprintf("target user %q does not exist on target", plan.User)
			check.Stderr = strings.TrimSpace(strings.TrimSpace(check.Stderr) + "\n" + msg)
			if check.ExitCode == 0 {
				check.ExitCode = 1
			}
			return check, true, nil
		}
	}

	if method == privilege.MethodSU && plan.NonInteractiveRun != "" {
		run, runErr := exec.Execute(ctx, plan.NonInteractiveRun)
		if runErr == nil && run.ExitCode == 0 {
			return run, true, nil
		}
	}

	if method == privilege.MethodSudo && plan.ValidateCommand != "" {
		validate, _ := exec.Execute(ctx, plan.ValidateCommand)
		switch privilege.ClassifyValidateResult(validate) {
		case privilege.ValidateAllowed:
			run, runErr := exec.Execute(ctx, plan.NonInteractiveRun)
			return run, true, runErr
		case privilege.ValidateDenied:
			return validate, true, nil
		}
	}

	ie, ok := exec.(executor.InteractiveExecutor)
	if !ok {
		if plan.ProfileFallbackRun != "" {
			run, runErr := exec.Execute(ctx, plan.ProfileFallbackRun)
			if runErr == nil && run.ExitCode == 0 {
				run.Stdout = "[Profile Fallback: ran as current user with target user's environment]\n" + run.Stdout
				return run, true, nil
			}
		}
		if method == privilege.MethodSudo && plan.NonInteractiveRun != "" {
			run, runErr := exec.Execute(ctx, plan.NonInteractiveRun)
			return run, true, runErr
		}
		return executor.ExecResult{}, true, fmt.Errorf("interactive execution is required for %s but executor does not support it", method)
	}

	target := exec.Target()
	if strings.TrimSpace(target) == "" {
		target = "localhost"
	}
	pw, _ := cache.Get(target, method, plan.User)
	if strings.TrimSpace(pw) == "" {
		if prompter == nil {
			return executor.ExecResult{}, true, fmt.Errorf("no privilege prompt handler available")
		}
		resp, err := prompter.RequestPassword(ctx, privilege.PromptRequest{Target: target, Method: method, User: plan.User})
		if err != nil {
			return executor.ExecResult{}, true, err
		}
		if resp.Abort {
			return executor.ExecResult{}, true, fmt.Errorf("privilege prompt aborted")
		}
		pw = resp.Password
	}

	router := newStdinRouter(exec)
	var chunks []string
	passwordInjected := false
	requirePTY := plan.RequirePTY

	session, err := ie.ExecuteInteractive(ctx, executor.InteractiveSpec{Command: plan.InteractiveRun, RequirePTY: requirePTY}, func(chunk string) {
		chunks = append(chunks, chunk)
		if trimmed := strings.TrimSpace(chunk); trimmed != "" && trimmed != plan.PromptToken {
			EmitOutput(ctx, trimmed)
		}
		if passwordInjected {
			return
		}
		combined := strings.Join(chunks, "")
		if privilege.CountPromptMatches(method, combined, plan.PromptToken) > 0 {
			passwordInjected = true
			_ = router.Inject(pw)
		}
	})
	if err != nil {
		return executor.ExecResult{}, true, err
	}
	if err := router.BindSession(session); err != nil {
		return executor.ExecResult{}, true, err
	}

	run, waitErr := session.Wait()
	if waitErr != nil {
		return executor.ExecResult{}, true, waitErr
	}

	combined := strings.Join(chunks, "")
	run.Stdout = privilege.SanitizeOutput(method, combined, plan.PromptToken)
	if privilege.IsAuthFailure(combined, privilege.CountPromptMatches(method, combined, plan.PromptToken)) {
		cache.Delete(target, method, plan.User)
	}
	if run.ExitCode == 0 {
		cache.Set(target, method, plan.User, pw)
	}
	return run, true, nil
}
