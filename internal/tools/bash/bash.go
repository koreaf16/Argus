package bash

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/koreaf16/argus/internal/services/workspace"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/shellsignal"
	"github.com/koreaf16/argus/internal/types"
	"github.com/koreaf16/argus/internal/utils"
	ibash "github.com/koreaf16/argus/internal/utils/bash"
	"github.com/koreaf16/argus/internal/utils/permissions"
	"github.com/koreaf16/argus/internal/utils/shell"
)

type BashTool struct {
	provider shell.ShellProvider
}

func NewBashTool() *BashTool {
	return &BashTool{}
}

func (t *BashTool) Name() string {
	return "bash"
}

func (t *BashTool) IsVisible(ctx tool.Context) bool {
	targetInfo, err := tool.ResolveShellTargetInfo(ctx, "", false)
	if err != nil {
		return true
	}
	if targetInfo.Platform == workspace.PlatformWindows {
		return targetInfo.BashAvailable // WSL/Git Bash가 설치된 경우에만 표시
	}
	return true // Unix 또는 플랫폼 미확인 시 표시 (실행 시점에 실패 처리)
}

func (t *BashTool) Description(ctx tool.Context) string {
	guardPrefix := "Shell Guard: choose account with `as_user`; never put sudo/su inside `command`. "
	base := "bash 셸 명령을 실행합니다. " +
		"중요: 비밀번호, URL 또는 '!', '&'와 같은 특수 문자가 포함된 문자열에는 반드시 작은따옴표(')를 사용하여 셸 확장을 방지하세요. "
	if tool.RequiresExplicitServerAlias(ctx) {
		aliases := tool.RegisteredWorkspaceAliases(ctx)
		base += "여러 워크스페이스가 등록되어 있습니다 (" + strings.Join(aliases, ", ") + "). `server` 매개변수가 필수입니다. "
	}
	base += "\n\n멀티 채널 및 권한 상승:\n" +
		"- 이 bash 도구는 멀티 채널 SSH 백본을 통해 명령을 라우팅합니다. 명령은 (server, role, channel) 세 쌍으로 격리됩니다.\n" +
		"  환경이나 현재 디렉토리를 공유하지 않고 작업을 병렬로 실행하려면 서로 다른 역할/채널을 사용하세요.\n" +
		"- 명령이 sudo/su -로 시작하면 라우터가 권한 전용 레인(예: alias|user>root)을 선택하거나 생성합니다.\n" +
		"  동일한 역할/채널의 후속 명령은 해당 레인에 유지됩니다.\n" +
		"- sudo/su 명령으로 이 도구를 호출하기 전에, 워크스페이스 설정에서 대상 서버의 권한 상승(elevation)이\n" +
		"  활성화(ENABLED)되어 있는지 확인해야 합니다. 비활성화된 경우 작업을 중단하고 사용자를 안내하세요.\n" +
		"- 지속적인 권한 레인에 진입하려면 `sudo -i`, `sudo -i -u <user>` 또는 `su - <user>`를 사용하세요.\n" +
		"  일회성 권한 명령을 실행하려면(레인 전환 없음) `sudo -u <user> <body>`를 사용하세요."
	base += "\n\nShell Guard rule: do not put sudo/su inside `command`; set `as_user` (for example `root`) and send only the command body."
	return guardPrefix + base
}

func (t *BashTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "실행할 bash 명령",
			},
			"timeout_ms": map[string]any{
				"type":        "integer",
				"description": "선택 사항: 제한 시간(밀리초)",
			},
			"workdir": map[string]any{
				"type":        "string",
				"description": "선택 사항: 작업 디렉토리 (허용된 루트 내부여야 함)",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "선택 사항: 명령에 대한 짧은 요약",
			},
			"server": map[string]any{
				"type":        "string",
				"description": "선택 사항: 워크스페이스 별칭. 기본값은 활성 워크스페이스입니다.",
			},
			"role": map[string]any{
				"type":        "string",
				"description": "선택 사항: 워크플로우 역할 (예: source_db, target_app). 세션 상태(PTY/CWD/ENV)를 격리하는 데 사용됩니다.",
			},
			"channel": map[string]any{
				"type":        "string",
				"description": "선택 사항: 워크플로우 채널 (source, target, transfer, verify). 세션 상태를 격리하는 데 사용됩니다.",
			},
			"password": map[string]any{
				"type":        "string",
				"description": "선택 사항: 원격 서버 SSH 인증을 위한 비밀번호.",
			},
			"root_password": map[string]any{
				"type":        "string",
				"description": "선택 사항: 권한 상승(sudo 또는 su)을 위한 비밀번호.",
			},
			"as_user": map[string]any{
				"type":        "string",
				"description": "Execution account for this command. Use this instead of inline sudo/su; Shell Guard selects or creates the matching account channel.",
			},
			"privilege_method": map[string]any{
				"type":        "string",
				"description": "Optional account switch method for as_user: sudo or su.",
			},
			"reuse_session": map[string]any{
				"type":        "string",
				"description": "권장되지 않음. 레인은 호스트별로 항상 재사용됩니다.",
			},
			"session_id": map[string]any{
				"type":        "string",
				"description": "권장되지 않음. 레인은 호스트 별칭으로 식별되며 계정 스택은 레인 내부에서 추적됩니다.",
			},
			"pty_mode": map[string]any{
				"type":        "string",
				"description": "권장되지 않음. 레인은 원격 셸을 위해 항상 PTY를 할당합니다.",
			},
			"background": map[string]any{
				"type":        "boolean",
				"description": "true면 즉시 백그라운드 잡으로 시작하고 background_task_id를 반환합니다(자동 5초 대기 없음). false면 포그라운드를 유지하며 자동 백그라운드 전환도 비활성화됩니다. 생략하면 오래 걸리는 명령의 경우 5초 후 자동으로 백그라운드로 전환됩니다.",
			},
			"stdin": map[string]any{
				"type":        "string",
				"description": "선택 사항: 명령 시작 시 stdin으로 한 번에 전달할 페이로드. 큰 텍스트(SQL 덤프, JSON, 패치 등)는 command 인라인 대신 이 필드를 사용하세요. command는 `psql -`, `jq .`, `cat`처럼 stdin을 읽는 reader로 작성합니다.",
			},
		},
		"required": []string{"command"},
	}
}

func (t *BashTool) IsReadOnly() bool {
	return false
}

// IsConcurrencySafe allows orchestration to run independent read-only shell
// commands in parallel while keeping mutating commands serialized.
func (t *BashTool) IsConcurrencySafe(input json.RawMessage) bool {
	command := tool.ExtractStringInput(input, "command")
	return tool.IsReadOnlyShellCommand(command, "bash")
}

func (t *BashTool) MaxResultSizeChars() int {
	return 100000
}

func (t *BashTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	events := make(chan tool.ToolEvent, 2)

	var req struct {
		Command         string `json:"command"`
		TimeoutMS       int    `json:"timeout_ms"`
		WorkDir         string `json:"workdir"`
		Server          string `json:"server"`
		Role            string `json:"role"`
		Channel         string `json:"channel"`
		Password        string `json:"password"`
		RootPassword    string `json:"root_password"`
		AsUser          string `json:"as_user"`
		PrivilegeMethod string `json:"privilege_method"`
		ReuseSession    string `json:"reuse_session"`
		SessionID       string `json:"session_id"`
		PTYMode         string `json:"pty_mode"`
		Background      *bool  `json:"background"`
		Stdin           string `json:"stdin"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	go func() {
		defer close(events)
		command := strings.TrimSpace(req.Command)
		if command == "" {
			events <- tool.NewErrorEvent(fmt.Errorf("명령어는 비어 있을 수 없습니다"))
			return
		}

		// ssh 직접 실행 차단: 워크스페이스 연결은 server_connect 도구를 사용해야 함
		if strings.HasPrefix(command, "ssh ") || command == "ssh" {
			events <- tool.NewErrorEvent(fmt.Errorf(
				"bash 도구에서는 직접적인 ssh 명령이 허용되지 않습니다. " +
					"워크스페이스에 연결하거나 전환하려면 서버 별칭과 함께 `server_connect` 도구를 사용하세요.",
			))
			return
		}

		if err := tool.ValidateTextOnlyCapable("bash", command); err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		if err := tool.ValidateCommandIntegrity("bash", command); err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		roleCtx, err := tool.ResolveExecutionRole(ctx, req.Server, req.Role, req.Channel, "bash")
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		req.Server = roleCtx.Server

		if err := tool.RequireExplicitServerAlias(ctx, req.Server, "bash"); err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}

		targetAlias, err := tool.ResolveValidatedWorkspaceAlias(ctx, req.Server)
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		targetInfo, err := tool.ResolveShellTargetInfo(ctx, req.Server, true)
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		if err := tool.ValidateShellCompatibility("bash", targetInfo); err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}

		isRemote := tool.IsRemoteWorkspace(ctx, targetAlias)

		// 시스템에 저장된 인증 정보(ID/PW) 자동 연동
		finalCommand := command
		execOpts := workspace.ExecOptions{
			Shell:           resolveBashLocalShell(targetInfo, isRemote),
			Password:        req.Password,
			RootPassword:    req.RootPassword,
			AsUser:          strings.TrimSpace(req.AsUser),
			PrivilegeMethod: strings.TrimSpace(req.PrivilegeMethod),
			ReuseSession:    strings.TrimSpace(req.ReuseSession),
			SessionID:       strings.TrimSpace(req.SessionID),
			PTYMode:         strings.TrimSpace(req.PTYMode),
			Role:            strings.TrimSpace(req.Role),
			Channel:         strings.TrimSpace(req.Channel),
		}
		tool.ApplyRoleDefaultsToExecOptions(&execOpts, roleCtx)

		// 채널 백본이 활성일 때는 sudo/su prefix를 strip하지 않는다.
		// channel.RouteFor가 명령 원문을 직접 분석해 권한 lane을 결정하기 때문에
		// prefix를 strip하면 라우팅 결정이 끊긴다 (예: "sudo whoami"가 단순
		// "whoami"로 변형되어 default lane에 잘못 들어감).
		if ctx.Workspace != nil && targetInfo.Platform == workspace.PlatformUnix && !channelBackboneActive() {
			currentUser := strings.TrimSpace(ctx.Workspace.CurrentUser(targetAlias))
			trimmedCmd := strings.TrimSpace(command)
			targetUser, cmdBody := parseTargetUser(trimmedCmd)
			if targetUser != "" && strings.TrimSpace(cmdBody) != "" {
				method := "sudo"
				if strings.HasPrefix(trimmedCmd, "su ") {
					method = "su"
				}
				if execOpts.AsUser == "" && !strings.EqualFold(targetUser, currentUser) {
					execOpts.AsUser = targetUser
					execOpts.PrivilegeMethod = method
					execOpts.ReuseSession = firstNonEmpty(execOpts.ReuseSession, workspace.ReuseSessionAuto)
					finalCommand = cmdBody
				} else if strings.EqualFold(targetUser, currentUser) || strings.EqualFold(targetUser, execOpts.AsUser) {
					finalCommand = cmdBody
					// wrapper가 strip된 이후에도 같은 사용자 shell을 재사용할 수 있도록
					// AsUser를 채워서 account shell 키에 포함시킨다.
					if execOpts.AsUser == "" {
						execOpts.AsUser = targetUser
					}
					execOpts.ReuseSession = firstNonEmpty(execOpts.ReuseSession, workspace.ReuseSessionAuto)
				}
			}
		}
		// 권한 채널 자동 재사용: compound 명령(예: `sudo X && sudo Y`)에서도
		// 매 호출마다 새 SSH exec를 여는 대신 캐시된 PTY 세션을 재사용한다.
		// 원격 Unix 세션의 경우 SSH 멀티플렉싱 효과를 극대화하기 위해 기본적으로 account shell을 사용한다.
		channelDecision := "single_exec"
		if targetInfo.Platform == workspace.PlatformUnix && isRemote &&
			!strings.EqualFold(execOpts.ReuseSession, workspace.ReuseSessionNever) &&
			!strings.EqualFold(execOpts.PTYMode, workspace.PTYNever) {
			execOpts.ReuseSession = workspace.ReuseSessionRequired
			if containsSudoToken(finalCommand) {
				if execOpts.PrivilegeMethod == "" {
					execOpts.PrivilegeMethod = workspace.PrivilegeSudo
				}
				channelDecision = "privileged_account_shell"
			} else {
				channelDecision = "account_shell"
			}
		} else if targetInfo.Platform == workspace.PlatformUnix && containsSudoToken(finalCommand) &&
			!strings.EqualFold(execOpts.ReuseSession, workspace.ReuseSessionNever) &&
			!strings.EqualFold(execOpts.PTYMode, workspace.PTYNever) {
			execOpts.ReuseSession = workspace.ReuseSessionRequired
			if execOpts.PrivilegeMethod == "" {
				execOpts.PrivilegeMethod = workspace.PrivilegeSudo
			}
			channelDecision = "privileged_account_shell"
		} else if wantsAccountShell(execOpts) {
			channelDecision = "account_shell"
		}
		if ctx.EmitTrace != nil {
			channelPriv := channelRoutingPrivilege(ctx, targetAlias, finalCommand, isRemote, targetInfo.Platform)
			ctx.EmitTrace("aidebug.shell_channel", "", map[string]any{
				"tool":               "bash",
				"alias":              targetAlias,
				"decision":           channelDecision,
				"shell":              execOpts.Shell,
				"reuse_session":      execOpts.ReuseSession,
				"privilege_method":   execOpts.PrivilegeMethod,
				"as_user":            execOpts.AsUser,
				"session_id":         execOpts.SessionID,
				"role":               execOpts.Role,
				"channel":            execOpts.Channel,
				"sudo_in_command":    containsSudoToken(finalCommand),
				"channel_privilege":  channelPriv,
				"channel_transition": channelRoutingTransition(finalCommand),
				"reject_code":        channelRoutingRejectCode(ctx, targetAlias, finalCommand, isRemote, targetInfo.Platform),
			})
		}

		if err := tool.ValidateRoleMutation(roleCtx, "bash", finalCommand, false); err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}

		if ctx.Workspace != nil && tool.IsRemoteWorkspace(ctx, targetAlias) {
			credentialTarget := tool.CredentialTargetUser(execOpts.AsUser, execOpts.PrivilegeMethod)
			credentialAlias := targetAlias
			if resolvedTarget, resolveErr := ctx.Workspace.ResolveExecutionTarget(targetAlias); resolveErr == nil {
				credentialAlias = resolvedTarget.HostAlias
				if resolvedTarget.IsAccount && credentialTarget == "" {
					credentialTarget = resolvedTarget.TargetUser
				}
			}
			if req.Password == "" {
				req.Password = ctx.Workspace.GetPassword(credentialAlias, "ssh")
			}
			if req.RootPassword == "" && credentialTarget != "" {
				req.RootPassword = ctx.Workspace.GetPasswordForTarget(credentialAlias, "sudo", credentialTarget)
			}
			if req.RootPassword == "" {
				req.RootPassword = ctx.Workspace.GetPassword(credentialAlias, "sudo")
			}
			if req.RootPassword == "" && req.Password != "" {
				req.RootPassword = req.Password
			}

			if req.Password != "" {
				ctx.Workspace.SetPassword(credentialAlias, "ssh", req.Password)
			}
			if req.RootPassword != "" {
				ctx.Workspace.SetPasswordForTarget(credentialAlias, "sudo", credentialTarget, req.RootPassword)
				ctx.Workspace.SetPasswordForTarget(credentialAlias, "su", credentialTarget, req.RootPassword)
				ctx.Workspace.SetPassword(credentialAlias, "sudo", req.RootPassword)
				ctx.Workspace.SetPassword(credentialAlias, "su", req.RootPassword)
			}
			execOpts.Password = req.Password
			execOpts.RootPassword = firstNonEmpty(req.RootPassword, req.Password)
		}

		if tool.IsRemoteWorkspace(ctx, targetAlias) {
			execOpts.WorkingDir = req.WorkDir
			execOpts.Timeout = clampShellTimeout(req.TimeoutMS)
			remoteCommand := finalCommand
			// 비밀번호 prompt 의존을 끊기 위해 모든 unquoted sudo 토큰에
			// `printf "%s\n" 'PW' | sudo -S` prefix를 사전 주입한다.
			// RootPassword가 없으면 기존 stdin 주입 경로(prompt 감지)에 폴백한다.
			if !channelBackboneActive() && execOpts.RootPassword != "" && containsSudoToken(remoteCommand) {
				remoteCommand = rewriteSudoTokens(remoteCommand, tool.POSIXShellQuote(execOpts.RootPassword))
			}
			forceBackground := req.Background != nil && *req.Background
			allowAutoBackground := req.Background == nil
			result, err := executeRemoteCommand(
				ctx,
				targetAlias,
				remoteCommand,
				req.TimeoutMS,
				events,
				forceBackground,
				allowAutoBackground,
				execOpts,
				req.Stdin,
			)
			if err != nil {
				events <- tool.NewErrorEvent(err)
				return
			}
			syncActiveTargetFromLane(ctx, targetAlias)
			if result.Stdout == "" && result.Code == 0 && !result.Interrupted {
				result.Stdout = "(command completed with no output)"
			}
			resJSON, _ := json.Marshal(result)
			events <- tool.NewOutputEvent(string(resJSON))
			events <- tool.NewDoneEvent()
			return
		}

		workDir, err := tool.ResolveWorkingDirectory(ctx, req.WorkDir)
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}

		if wantsAccountShell(execOpts) {
			execOpts.WorkingDir = workDir
			forceBackground := req.Background != nil && *req.Background
			allowAutoBackground := req.Background == nil
			result, err := executeRemoteCommand(
				ctx,
				targetAlias,
				finalCommand,
				req.TimeoutMS,
				events,
				forceBackground,
				allowAutoBackground,
				execOpts,
				req.Stdin,
			)
			if err != nil {
				events <- tool.NewErrorEvent(err)
				return
			}
			if result.Stdout == "" && result.Code == 0 && !result.Interrupted {
				result.Stdout = "(command completed with no output)"
			}
			resJSON, _ := json.Marshal(result)
			events <- tool.NewOutputEvent(string(resJSON))
			events <- tool.NewDoneEvent()
			return
		}

		if t.provider == nil {
			shellPath, err := utils.FindSuitableShell()
			if err != nil {
				events <- tool.NewErrorEvent(err)
				return
			}
			if strings.TrimSpace(shellPath) == "" {
				events <- tool.NewErrorEvent(fmt.Errorf("bash 셸을 찾을 수 없습니다"))
				return
			}
			p, err := shell.CreateBashShellProvider(ctx.Context, shellPath, ibash.CreateAndSaveSnapshot)
			if err != nil {
				events <- tool.NewErrorEvent(err)
				return
			}
			t.provider = p
		}

		opts := shell.ExecCommandOpts{
			ID: fmt.Sprintf("%d", time.Now().UnixNano()),
		}
		res, err := t.provider.BuildExecCommand(ctx.Context, command, opts)
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}

		forceBackground := req.Background != nil && *req.Background
		allowAutoBackground := req.Background == nil
		result, err := executeCommand(
			ctx,
			res.CommandString,
			req.TimeoutMS,
			workDir,
			events,
			targetAlias,
			forceBackground,
			allowAutoBackground,
			req.Stdin,
		)
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}

		if result.Stdout == "" && result.Code == 0 && !result.Interrupted {
			result.Stdout = "(command completed with no output)"
		}
		resJSON, _ := json.Marshal(result)
		events <- tool.NewOutputEvent(string(resJSON))
		events <- tool.NewDoneEvent()
	}()

	return events, nil
}

func (t *BashTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	rawServer := tool.ExtractStringInput(input, "server")
	rawRole := tool.ExtractStringInput(input, "role")
	rawChannel := tool.ExtractStringInput(input, "channel")
	roleCtx, err := tool.ResolveExecutionRole(ctx, rawServer, rawRole, rawChannel, "bash")
	if err != nil {
		return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: err.Error()}, nil
	}
	rawServer = roleCtx.Server
	alias, err := tool.ResolveValidatedWorkspaceAlias(ctx, rawServer)
	if err != nil {
		return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: err.Error()}, nil
	}
	command := tool.ExtractStringInput(input, "command")
	if err := tool.ValidateRoleMutation(roleCtx, "bash", command, false); err != nil {
		return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: err.Error()}, nil
	}

	targetInfo, err := tool.ResolveShellTargetInfo(ctx, rawServer, false)
	if err != nil {
		return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: err.Error()}, nil
	}
	if targetInfo.Platform != workspace.PlatformUnknown {
		if err := tool.ValidateShellCompatibility("bash", targetInfo); err != nil {
			return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: err.Error()}, nil
		}
	}

	normalizedInput := input
	if ctx.Workspace != nil && tool.IsRemoteWorkspace(ctx, alias) {
		currentUser := strings.TrimSpace(ctx.Workspace.CurrentUser(alias))
		if currentUser != "" {
			var req map[string]any
			if err := json.Unmarshal(input, &req); err == nil {
				if rawCmd, ok := req["command"].(string); ok {
					normalizedCmd := stripRedundantPrivilegeWrapper(rawCmd, currentUser)
					if normalizedCmd != strings.TrimSpace(rawCmd) {
						req["command"] = normalizedCmd
						if b, err := json.Marshal(req); err == nil {
							normalizedInput = b
						}
					}
				}
			}
		}
	}

	permCtx := permissions.NewDefaultPermissionContext()
	if ctx.State != nil {
		permCtx.Mode = ctx.State.GetPermissionMode()
	}
	return CheckBashPermissions(ctx, permCtx, normalizedInput)
}

const (
	shellAutoBackgroundAfter = 5 * time.Second
	maxShellTailChars        = 128 * 1024
)

func executeCommand(
	ctx tool.Context,
	command string,
	timeoutMS int,
	workDir string,
	events chan<- tool.ToolEvent,
	targetAlias string,
	forceBackground bool,
	allowAutoBackground bool,
	stdinPayload string,
) (utils.ExecResult, error) {
	shellPath, err := utils.FindSuitableShell()
	if err != nil {
		return utils.ExecResult{}, fmt.Errorf("셸 찾기 실패: %w", err)
	}
	if strings.TrimSpace(shellPath) == "" {
		return utils.ExecResult{}, fmt.Errorf("bash 셸을 찾을 수 없습니다")
	}

	timeout := clampShellTimeout(timeoutMS)

	runCtx := ctx.Context
	runTimeout := timeout
	if ctx.ShellJobs != nil {
		// Keep detached jobs alive even after the foreground request context ends.
		runCtx = context.Background()
		runTimeout = 0
	}

	cmd := utils.RunCommand(runCtx, shellPath, []string{"-c", command}, map[string]string{
		"PAGER":         "cat",
		"SYSTEMD_PAGER": "cat",
	}, workDir, runTimeout, utils.SupportsPTY())

	inputChan := make(chan string, 16)
	events <- tool.ToolEvent{
		Kind:          tool.ToolEventChunk,
		InputResponse: inputChan,
	}

	if stdinPayload != "" && cmd.Stdin != nil {
		// 큰 페이로드는 별도 고루틴에서 비동기로 흘려보내 select 루프 블로킹을 피한다.
		// stdin은 Close 하지 않는다 — 사용자의 후속 키 입력 경로를 보존하기 위함.
		go func(payload string) {
			_, _ = cmd.Stdin.Write([]byte(payload))
		}(stdinPayload)
	}

	var (
		outputBuffer    string
		lastAnalyzedPos int
		idleDuration    = 400 * time.Millisecond
		idleTimer       = time.NewTimer(idleDuration)
	)
	if !idleTimer.Stop() {
		<-idleTimer.C
	}

	if forceBackground {
		if result, ok := startLocalBackgroundJob(ctx, cmd, targetAlias, command, outputBuffer, true); ok {
			idleTimer.Stop()
			return result, nil
		}
	}

	var (
		timeoutCh <-chan time.Time
		timeoutT  *time.Timer
	)
	if timeout > 0 {
		timeoutT = time.NewTimer(timeout)
		timeoutCh = timeoutT.C
		defer timeoutT.Stop()
	}

	var (
		autoBgCh <-chan time.Time
		autoBgT  *time.Timer
	)
	if allowAutoBackground && ctx.ShellJobs != nil {
		autoBgT = time.NewTimer(shellAutoBackgroundAfter)
		autoBgCh = autoBgT.C
		defer autoBgT.Stop()
	}

	streamCh := cmd.Stream
	resultCh := cmd.Result

	for {
		select {
		case <-ctx.Context.Done():
			cmd.Kill()
			return utils.ExecResult{Interrupted: true, Code: 1}, ctx.Context.Err()

		case input := <-inputChan:
			if shellsignal.IsBackgroundRequest(input) {
				if result, ok := startLocalBackgroundJob(ctx, cmd, targetAlias, command, outputBuffer, true); ok {
					idleTimer.Stop()
					return result, nil
				}
				continue
			}
			if cmd.Stdin != nil {
				_, _ = cmd.Stdin.Write([]byte(input))
			}

		case chunk, ok := <-streamCh:
			if !ok {
				streamCh = nil
				idleTimer.Stop()
				continue
			}
			outputBuffer = appendShellTail(outputBuffer, chunk, maxShellTailChars)
			events <- tool.NewChunkEvent(chunk)
			idleTimer.Stop()
			idleTimer.Reset(idleDuration)
			// 출력이 들어오는 동안은 백그라운드 전환 억제
			if autoBgT != nil {
				autoBgT.Reset(shellAutoBackgroundAfter)
			}

		case <-idleTimer.C:
			if len(outputBuffer) > lastAnalyzedPos {
				currentTail := outputBuffer[lastAnalyzedPos:]
				if len(currentTail) > 1000 {
					currentTail = currentTail[len(currentTail)-1000:]
				}

				kind, prompt, ok := analyzeInteractionSituation(ctx, currentTail)
				if ok {
					lastAnalyzedPos = len(outputBuffer)
					trimmedPrompt := strings.TrimSpace(prompt)
					if trimmedPrompt == "" {
						trimmedPrompt = "Input:"
					}
					kindUpper := strings.ToUpper(strings.TrimSpace(kind))
					switch kindUpper {
					case "PASSWORD", "SUDO", "SU", "SSH":
						pwChan := make(chan string)
						events <- tool.ToolEvent{
							Kind:             tool.ToolEventPasswordPrompt,
							Prompt:           trimmedPrompt,
							PasswordResponse: pwChan,
						}

						select {
						case <-ctx.Context.Done():
							cmd.Kill()
							return utils.ExecResult{Interrupted: true, Code: 1}, ctx.Context.Err()
						case response := <-pwChan:
							if cmd.Stdin != nil {
								_, _ = cmd.Stdin.Write([]byte(response + "\n"))
							}
							idleTimer.Reset(idleDuration)
						}
					default:
						var value string
						if ctx.State != nil && ctx.State.InYoloMode() && ctx.ExecuteSubQuery != nil {
							// [YOLO] 자율 모드: LLM 이 입력값을 결정
							events <- tool.NewChunkEvent(fmt.Sprintf("\n[YOLO] 자율 모드: 명령어가 입력을 요청함 (%s). LLM 이 판단 중...\n", trimmedPrompt))
							sysPrompt := "You are a senior systems engineer. A bash command is asking for input. Decide what to type to proceed safely and effectively. Return ONLY the text to be typed."
							usrPrompt := fmt.Sprintf("Command output so far:\n%s\n\nPrompt: %s\nInput Type: %s\nWhat should I type?", string(outputBuffer), trimmedPrompt, kindUpper)
							resp, err := ctx.ExecuteSubQuery(ctx.Context, sysPrompt, usrPrompt)
							if err == nil {
								value = strings.TrimSpace(resp)
							} else {
								value = "" // fallback
							}
						} else {
							askChan := make(chan tool.AskUserResponse)
							question := &tool.AskUserQuestion{
								Question: trimmedPrompt,
								Type:     "text",
							}
							if kindUpper == "YES_NO" {
								question.Type = "yesno"
								question.Options = []tool.AskUserOption{{Value: "yes", Label: "Yes"}, {Value: "no", Label: "No"}}
								question.Default = "no"
							}
							events <- tool.ToolEvent{
								Kind:            tool.ToolEventAskUserPrompt,
								Question:        question,
								AskUserResponse: askChan,
							}

							select {
							case <-ctx.Context.Done():
								cmd.Kill()
								return utils.ExecResult{Interrupted: true, Code: 1}, ctx.Context.Err()
							case response := <-askChan:
								value = strings.TrimSpace(response.Value)
							}
						}

						if value == "" && kindUpper == "YES_NO" {
							value = "no"
						}
						if cmd.Stdin != nil {
							_, _ = cmd.Stdin.Write([]byte(value + "\n"))
						}
						idleTimer.Reset(idleDuration)
					}
				} else {
					lastAnalyzedPos = len(outputBuffer)
				}
			}

		case res := <-resultCh:
			idleTimer.Stop()
			if res.ActiveUser == "" {
				res.ActiveUser = os.Getenv("USERNAME")
				if res.ActiveUser == "" {
					res.ActiveUser = os.Getenv("USER")
				}
			}
			if res.ActiveCWD == "" {
				res.ActiveCWD = workDir
			}
			if res.ActiveLane == "" {
				res.ActiveLane = "local"
			}
			return res, nil

		case <-autoBgCh:
			if result, ok := startLocalBackgroundJob(ctx, cmd, targetAlias, command, outputBuffer, false); ok {
				idleTimer.Stop()
				return result, nil
			}

		case <-timeoutCh:
			cmd.Kill()
			return utils.ExecResult{Code: 1, Stderr: "command timed out"}, fmt.Errorf("timeout")
		}
	}
}

func executeRemoteCommand(
	ctx tool.Context,
	targetAlias string,
	command string,
	timeoutMS int,
	events chan<- tool.ToolEvent,
	forceBackground bool,
	allowAutoBackground bool,
	execOpts workspace.ExecOptions,
	stdinPayload string,
) (utils.ExecResult, error) {
	if ctx.Workspace == nil {
		return utils.ExecResult{}, fmt.Errorf("workspace manager is unavailable")
	}

	timeout := clampShellTimeout(timeoutMS)

	runCtx := ctx.Context
	if ctx.ShellJobs != nil {
		runCtx = context.Background()
	}

	var subQuery workspace.ExecuteSubQueryFunc
	if ctx.ExecuteSubQuery != nil {
		subQuery = func(c context.Context, sys, user string) (string, error) {
			return ctx.ExecuteSubQuery(c, sys, user)
		}
	}

	handle, err := ctx.Workspace.StartExecWithOptions(
		runCtx,
		targetAlias,
		command,
		execOpts,
		subQuery,
	)
	if err != nil {
		return utils.ExecResult{}, err
	}

	inputChan := make(chan string, 16)
	events <- tool.ToolEvent{
		Kind:          tool.ToolEventChunk,
		InputResponse: inputChan,
	}

	if stdinPayload != "" && handle.Write != nil {
		// 원격 핸들도 동일하게 별도 고루틴으로 비동기 write.
		go func(payload string) {
			_ = handle.Write(payload)
		}(stdinPayload)
	}

	var outputBuffer string
	streamCh := handle.Stream
	resultCh := handle.Result

	if forceBackground {
		if result, ok := startRemoteBackgroundJob(ctx, handle, streamCh, resultCh, targetAlias, command, outputBuffer, true); ok {
			return result, nil
		}
	}

	var (
		timeoutCh <-chan time.Time
		timeoutT  *time.Timer
	)
	if timeout > 0 {
		timeoutT = time.NewTimer(timeout)
		timeoutCh = timeoutT.C
		defer timeoutT.Stop()
	}

	var (
		autoBgCh <-chan time.Time
		autoBgT  *time.Timer
	)
	if allowAutoBackground && ctx.ShellJobs != nil {
		autoBgT = time.NewTimer(shellAutoBackgroundAfter)
		autoBgCh = autoBgT.C
		defer autoBgT.Stop()
	}

	for {
		select {
		case <-ctx.Context.Done():
			if handle.Kill != nil {
				handle.Kill()
			}
			return utils.ExecResult{Interrupted: true, Code: 1}, ctx.Context.Err()

		case input := <-inputChan:
			if shellsignal.IsBackgroundRequest(input) {
				if result, ok := startRemoteBackgroundJob(ctx, handle, streamCh, resultCh, targetAlias, command, outputBuffer, true); ok {
					return result, nil
				}
				continue
			}
			if handle.Write != nil {
				_ = handle.Write(input)
			}

		case chunk, ok := <-streamCh:
			if !ok {
				streamCh = nil
				continue
			}
			outputBuffer = appendShellTail(outputBuffer, chunk, maxShellTailChars)
			events <- tool.NewChunkEvent(chunk)
			// 출력이 들어오는 동안은 백그라운드 전환 억제
			if autoBgT != nil {
				autoBgT.Reset(shellAutoBackgroundAfter)
			}

		case res, ok := <-resultCh:
			if !ok {
				return utils.ExecResult{Code: 1, Stderr: "remote command ended unexpectedly"}, fmt.Errorf("remote command ended unexpectedly")
			}
			laneKey := "default"
			if ctx.Workspace != nil {
				priv := ctx.Workspace.ActivePrivilege(targetAlias)
				if priv != "" {
					laneKey = string(priv)
				}
			}
			return utils.ExecResult{
				Stdout:     res.Stdout,
				Stderr:     res.Stderr,
				Code:       res.Code,
				ActiveLane: laneKey,
				ActiveUser: res.User,
				ActiveCWD:  res.CWD,
			}, nil

		case <-autoBgCh:
			if result, ok := startRemoteBackgroundJob(ctx, handle, streamCh, resultCh, targetAlias, command, outputBuffer, false); ok {
				return result, nil
			}

		case <-timeoutCh:
			if handle.Kill != nil {
				handle.Kill()
			}
			return utils.ExecResult{Code: 1, Stderr: "command timed out"}, fmt.Errorf("timeout")
		}
	}
}

func startLocalBackgroundJob(
	ctx tool.Context,
	cmd *utils.ShellCommand,
	targetAlias string,
	command string,
	initialOutput string,
	byUser bool,
) (utils.ExecResult, bool) {
	if ctx.ShellJobs == nil || cmd == nil {
		return utils.ExecResult{}, false
	}

	var writeFn func(string) error
	if cmd.Stdin != nil {
		writeFn = cmd.Write
	}
	killFn := func() {
		if cmd.Kill != nil {
			cmd.Kill()
		}
	}
	snap := ctx.ShellJobs.StartJob("bash", targetAlias, command, writeFn, killFn, initialOutput)
	jobID := snap.ID

	streamCh := cmd.Stream
	resultCh := cmd.Result
	go func() {
		for {
			select {
			case chunk, ok := <-streamCh:
				if !ok {
					streamCh = nil
					continue
				}
				ctx.ShellJobs.AppendOutput(jobID, chunk)
			case res := <-resultCh:
				errText := strings.TrimSpace(res.Stderr)
				if strings.TrimSpace(res.PreSpawnError) != "" {
					errText = strings.TrimSpace(res.PreSpawnError)
				}
				if res.Interrupted && errText == "" {
					errText = "interrupted"
				}
				ctx.ShellJobs.Complete(jobID, res.Code, errText)
				return
			}
		}
	}()

	return utils.ExecResult{
		Code:                      0,
		BackgroundTaskID:          jobID,
		OutputTaskID:              jobID,
		BackgroundedByUser:        byUser,
		AssistantAutoBackgrounded: !byUser,
	}, true
}

func startRemoteBackgroundJob(
	ctx tool.Context,
	handle *workspace.ExecHandle,
	streamCh <-chan string,
	resultCh <-chan workspace.ExecResult,
	targetAlias string,
	command string,
	initialOutput string,
	byUser bool,
) (utils.ExecResult, bool) {
	if ctx.ShellJobs == nil || handle == nil {
		return utils.ExecResult{}, false
	}

	var writeFn func(string) error
	if handle.Write != nil {
		writeFn = handle.Write
	}
	killFn := func() {
		if handle.Kill != nil {
			handle.Kill()
		}
	}
	snap := ctx.ShellJobs.StartJob("bash", targetAlias, command, writeFn, killFn, initialOutput)
	jobID := snap.ID

	go func() {
		localStream := streamCh
		localResult := resultCh
		for {
			select {
			case chunk, ok := <-localStream:
				if !ok {
					localStream = nil
					continue
				}
				ctx.ShellJobs.AppendOutput(jobID, chunk)
			case res, ok := <-localResult:
				if !ok {
					ctx.ShellJobs.Complete(jobID, 1, "remote command ended unexpectedly")
					return
				}
				ctx.ShellJobs.Complete(jobID, res.Code, strings.TrimSpace(res.Stderr))
				return
			}
		}
	}()

	return utils.ExecResult{
		Code:                      0,
		BackgroundTaskID:          jobID,
		OutputTaskID:              jobID,
		BackgroundedByUser:        byUser,
		AssistantAutoBackgrounded: !byUser,
	}, true
}

func clampShellTimeout(timeoutMS int) time.Duration {
	timeout := 10 * time.Minute
	if timeoutMS > 0 {
		timeout = time.Duration(timeoutMS) * time.Millisecond
	}
	if timeout > 30*time.Minute {
		timeout = 30 * time.Minute
	}
	return timeout
}

func appendShellTail(current, chunk string, maxChars int) string {
	merged := current + chunk
	if maxChars <= 0 || len(merged) <= maxChars {
		return merged
	}
	return merged[len(merged)-maxChars:]
}

// resolveBashLocalShell picks the effective shell for bash tool execution.
// Windows + local workspace falls back to powershell because Windows lacks
// bash by default and cmd/PowerShell builtins (dir, type, Get-*, $env:) are
// what naturally exists there. Remote workspaces keep "bash" — SSH targets
// resolve their own shell via opts.Shell at the ssh_session layer.
func resolveBashLocalShell(targetInfo tool.ShellTargetInfo, isRemote bool) string {
	if !isRemote && targetInfo.Platform == workspace.PlatformWindows {
		return "powershell"
	}
	return "bash"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func wantsAccountShell(opts workspace.ExecOptions) bool {
	if strings.EqualFold(strings.TrimSpace(opts.ReuseSession), workspace.ReuseSessionNever) {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(opts.PTYMode), workspace.PTYNever) {
		return false
	}
	return strings.TrimSpace(opts.AsUser) != "" ||
		strings.TrimSpace(opts.SessionID) != "" ||
		strings.EqualFold(strings.TrimSpace(opts.ReuseSession), workspace.ReuseSessionRequired)
}

func analyzeInteractionSituation(ctx tool.Context, tail string) (kind, prompt string, ok bool) {
	if ctx.ExecuteSubQuery == nil {
		// Fallback to basic detection if LLM is unavailable
		return detectLocalPasswordPrompt(tail)
	}

	systemPrompt := `You are an expert system monitor. Analyze the provided terminal output tail and determine if the process is waiting for user input.
Respond ONLY with a JSON object in the following format:
{
  "is_waiting": true/false,
  "type": "PASSWORD" | "CHOICE" | "YES_NO" | "TEXT",
  "prompt_label": "Short label for the input field (e.g. 'sudo password', 'Continue? [Y/n]')"
}
If is_waiting is false, other fields can be empty.`

	userPrompt := fmt.Sprintf("Terminal output tail:\n---\n%s\n---", tail)

	resp, err := ctx.ExecuteSubQuery(ctx.Context, systemPrompt, userPrompt)
	if err != nil {
		return "", "", false
	}

	var result struct {
		IsWaiting   bool   `json:"is_waiting"`
		Type        string `json:"type"`
		PromptLabel string `json:"prompt_label"`
	}

	// Try to find JSON in the response
	jsonStr := resp
	if start := strings.Index(resp, "{"); start != -1 {
		if end := strings.LastIndex(resp, "}"); end != -1 {
			jsonStr = resp[start : end+1]
		}
	}

	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return "", "", false
	}

	if !result.IsWaiting {
		return "", "", false
	}

	return result.Type, result.PromptLabel, true
}

func detectLocalPasswordPrompt(output string) (kind, prompt string, ok bool) {
	tail := output
	if len(tail) > 256 {
		tail = tail[len(tail)-256:]
	}
	lower := strings.ToLower(tail)
	switch {
	case strings.Contains(lower, "[sudo] password for "):
		return "sudo", "sudo password:", true
	case strings.Contains(lower, "su password:"):
		return "su", "su password:", true
	case strings.Contains(lower, "'s password:"):
		return "ssh", "ssh password:", true
	case strings.Contains(lower, "password:"):
		return "password", "password:", true
	default:
		return "", "", false
	}
}
