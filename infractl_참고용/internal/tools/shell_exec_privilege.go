// Package tools
// File: shell_exec_privilege.go
// Description: 권한 승격 계획 수립 및 인라인 sudo/su 실행 경로
// Responsibility: preparePrivilegeExecution + runCommand + 승인된 인라인 wrapper 실행

package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/privilege"
)

// preparePrivilegeExecution은 명령을 정규화하고 인라인 권한 래퍼를 탐지한다.
// 인라인 sudo/su 감지 시 사용자 확인을 거친 후 실행을 승인한다.
func (t *ShellExecTool) preparePrivilegeExecution(
	ctx context.Context,
	command string,
	explicitMethod string,
	explicitUser string,
) (privilegeExecutionPlan, *ToolOutcome, error) {
	normalized, normErr := NormalizePrivilegeCommand(command, explicitMethod, explicitUser)
	if normErr == nil {
		return privilegeExecutionPlan{normalized: normalized}, nil, nil
	}

	if !errors.Is(normErr, errEmbeddedPrivilegeWrapper) {
		msg := fmt.Sprintf("Pre-check failed: %s", normErr)
		outcome := ToolOutcome{Content: msg, Success: false, ExitCode: 1, ErrorMessage: msg}
		return privilegeExecutionPlan{}, &outcome, nil
	}

	// Embedded privilege wrapper: require explicit user confirmation.
	resp, qErr := RequestQuestion(ctx, QuestionRequest{
		Header:   "Security Confirmation",
		Question: fmt.Sprintf("Command contains an inline privilege wrapper (sudo/su inside a pipe or compound expression). This bypasses managed privilege handling.\n\nCommand: %s\n\nRun anyway?", command),
		Options:  []QuestionOption{{Label: "Run Anyway"}, {Label: "Cancel"}},
	})
	if qErr != nil {
		msg := "embedded privilege wrapper detected: use become_method parameter instead of inline sudo/su"
		return privilegeExecutionPlan{}, &ToolOutcome{Content: msg, Success: false, ExitCode: 1, ErrorMessage: msg}, nil
	}
	if resp.SelectedLabel != "Run Anyway" {
		msg := "user declined embedded privilege wrapper execution"
		return privilegeExecutionPlan{}, &ToolOutcome{Content: msg, Success: false, ExitCode: 1, ErrorMessage: msg}, nil
	}
	return privilegeExecutionPlan{
		normalized:        NormalizedPrivilegeCommand{Command: command},
		rawInlineApproved: true,
		inline:            bestEffortEmbeddedPrivilegeSpec(command),
	}, nil, nil
}

func requestsPrivilegeEscalation(command, explicitMethod string) bool {
	if strings.TrimSpace(explicitMethod) != "" {
		return true
	}
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return false
	}
	return leadingPrivilegeWrapperRe.MatchString(cmd) || hasEmbeddedPrivilegeWrapper(cmd)
}

func bestEffortEmbeddedPrivilegeSpec(command string) *embeddedPrivilegeSpec {
	spec, err := findEmbeddedPrivilegeSpec(command)
	if err != nil {
		return nil
	}
	return spec
}

// runCommand는 승격 경로 우선순위에 따라 실제 실행을 수행한다.
// 1) 이미 획득한 권한 세션 재사용 → 2) become wrapper → 3) 인라인 승인 래퍼 → 4) 직접 실행 (+ permission 재시도)
func (t *ShellExecTool) runCommand(ctx context.Context, exec executor.Executor, cmd, becomeMethod, becomeUser string, inline *embeddedPrivilegeSpec) (executor.ExecResult, error) {
	if becomeMethod != "" {
		if result, ok := executeWithAcquiredPrivilege(ctx, exec, cmd, becomeUser); ok {
			return result, nil
		}
		if result, ok, err := executeWithBecome(ctx, exec, cmd, becomeMethod, becomeUser, t.PrivilegeCache, t.PromptHandler); ok || err != nil {
			return result, err
		}
	}
	if inline != nil {
		if result, ok, err := executeApprovedInlinePrivilege(ctx, exec, cmd, inline.Method, inline.User, t.PrivilegeCache, t.PromptHandler); ok || err != nil {
			return result, err
		}
	}

	result, err := t.executeDirect(ctx, exec, cmd)
	if err != nil {
		return result, err
	}

	if becomeMethod == "" && isPermissionFailure(result, nil) {
		if retried, ok := executePlainViaAcquiredRoot(ctx, exec, cmd); ok {
			return retried, nil
		}
	}

	return result, nil
}

// executeApprovedInlinePrivilege는 사용자가 명시적으로 승인한 인라인 sudo/su 래퍼를 실행한다.
// PTY 대화형 세션을 열고 비밀번호 프롬프트를 감지하여 캐시된 비밀번호를 주입한다.
func executeApprovedInlinePrivilege(
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
		return executor.ExecResult{}, false, nil
	}

	ie, ok := exec.(executor.InteractiveExecutor)
	if !ok {
		return executor.ExecResult{}, false, nil
	}

	target := exec.Target()
	if strings.TrimSpace(target) == "" {
		target = "localhost"
	}
	user = privilege.NormalizeUser(user)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	pw, _ := cache.Get(target, method, user)
	router := newStdinRouter(exec)
	var (
		chunks       []string
		prompted     bool
		promptErr    error
		promptErrMu  sync.Mutex
		passwordUsed string
	)

	setPromptErr := func(err error) {
		if err == nil {
			return
		}
		promptErrMu.Lock()
		if promptErr == nil {
			promptErr = err
			cancel()
		}
		promptErrMu.Unlock()
	}

	session, err := ie.ExecuteInteractive(runCtx, executor.InteractiveSpec{Command: command, RequirePTY: true}, func(chunk string) {
		chunks = append(chunks, chunk)
		if trimmed := strings.TrimSpace(chunk); trimmed != "" {
			EmitOutput(ctx, trimmed)
		}
		if prompted {
			return
		}
		combined := strings.Join(chunks, "")
		if privilege.CountPromptMatches(method, combined, "") == 0 {
			return
		}
		prompted = true
		passwordUsed = pw
		if strings.TrimSpace(passwordUsed) == "" {
			if prompter == nil {
				setPromptErr(fmt.Errorf("no privilege prompt handler available"))
				return
			}
			resp, err := prompter.RequestPassword(ctx, privilege.PromptRequest{Target: target, Method: method, User: user})
			if err != nil {
				setPromptErr(err)
				return
			}
			if resp.Abort {
				setPromptErr(fmt.Errorf("privilege prompt aborted"))
				return
			}
			passwordUsed = resp.Password
		}
		if err := router.Inject(passwordUsed); err != nil {
			setPromptErr(err)
		}
	})
	if err != nil {
		return executor.ExecResult{}, true, err
	}
	if err := router.BindSession(session); err != nil {
		return executor.ExecResult{}, true, err
	}

	run, waitErr := session.Wait()
	promptErrMu.Lock()
	localPromptErr := promptErr
	promptErrMu.Unlock()
	if localPromptErr != nil {
		return executor.ExecResult{}, true, localPromptErr
	}
	if waitErr != nil {
		return executor.ExecResult{}, true, waitErr
	}

	combined := strings.Join(chunks, "")
	run.Stdout = privilege.SanitizeOutput(method, combined, "")
	if privilege.IsAuthFailure(combined, privilege.CountPromptMatches(method, combined, "")) {
		cache.Delete(target, method, user)
	}
	if run.ExitCode == 0 && strings.TrimSpace(passwordUsed) != "" {
		cache.Set(target, method, user, passwordUsed)
	}
	return run, true, nil
}
