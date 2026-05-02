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
	base := "Execute bash shell commands. " +
		"CRITICAL: Always use SINGLE QUOTES (') for passwords, URLs, or any string with special characters like '!', '&', etc. to avoid shell expansion. "
	if tool.RequiresExplicitServerAlias(ctx) {
		aliases := tool.RegisteredWorkspaceAliases(ctx)
		base += "Multiple workspaces are registered (" + strings.Join(aliases, ", ") + "); the `server` parameter is REQUIRED. "
	}
	base += "\n\nMULTI-CHANNEL & PRIVILEGE ESCALATION:\n" +
		"- This bash tool routes commands through a multi-channel SSH backbone. Commands are isolated\n" +
		"  by (server, role, channel) triplets. Use different roles/channels to run tasks in parallel\n" +
		"  without sharing environment or current directory.\n" +
		"- When the command starts with sudo/su -, the router picks or creates a privilege-specific\n" +
		"  lane (e.g. alias|user>root). Subsequent commands on the same role/channel will stay in that lane.\n" +
		"- BEFORE calling this tool with a sudo/su command, you MUST verify the target server's\n" +
		"  elevation is ENABLED in the workspace block. If DISABLED, stop and guide the user.\n" +
		"- To enter a persistent privilege lane, use `sudo -i`, `sudo -i -u <user>`, or `su - <user>`.\n" +
		"  To run a one-shot privileged command (no lane switch), use `sudo -u <user> <body>`."
	return base
}

func (t *BashTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The bash command to execute",
			},
			"timeout_ms": map[string]any{
				"type":        "integer",
				"description": "Optional timeout in milliseconds",
			},
			"workdir": map[string]any{
				"type":        "string",
				"description": "Optional working directory (must be inside allowed roots)",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Optional short summary of the command",
			},
			"server": map[string]any{
				"type":        "string",
				"description": "Optional workspace alias. Defaults to active workspace.",
			},
			"role": map[string]any{
				"type":        "string",
				"description": "Optional workflow role (e.g. source_db, target_app). Used to isolate session state (PTY/CWD/ENV).",
			},
			"channel": map[string]any{
				"type":        "string",
				"description": "Optional workflow channel (source, target, transfer, verify). Used to isolate session state.",
			},
			"password": map[string]any{
				"type":        "string",
				"description": "Optional password for remote server SSH authentication.",
			},
			"root_password": map[string]any{
				"type":        "string",
				"description": "Optional password for privilege escalation (sudo or su).",
			},
			"as_user": map[string]any{
				"type":        "string",
				"description": "DEPRECATED. Send `su - <user>` or `sudo -i -u <user>` as the command instead — the lane tracks the account stack automatically.",
			},
			"privilege_method": map[string]any{
				"type":        "string",
				"description": "DEPRECATED. Use `sudo` / `su` directly in the command.",
			},
			"reuse_session": map[string]any{
				"type":        "string",
				"description": "DEPRECATED. The lane is always reused per host.",
			},
			"session_id": map[string]any{
				"type":        "string",
				"description": "DEPRECATED. Lanes are keyed by host alias; account stacks are tracked inside the lane.",
			},
			"pty_mode": map[string]any{
				"type":        "string",
				"description": "DEPRECATED. The lane always allocates a PTY for remote shells.",
			},
			"background": map[string]any{
				"type":        "boolean",
				"description": "Run as a background job. Omit to auto-background after a few seconds for long-running commands.",
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
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	go func() {
		defer close(events)
		command := strings.TrimSpace(req.Command)
		if command == "" {
			events <- tool.NewErrorEvent(fmt.Errorf("command cannot be empty"))
			return
		}

		// ssh 직접 실행 차단: 워크스페이스 연결은 server_connect 도구를 사용해야 함
		if strings.HasPrefix(command, "ssh ") || command == "ssh" {
			events <- tool.NewErrorEvent(fmt.Errorf(
				"direct ssh commands are not allowed in the bash tool. " +
					"To connect to or switch to a workspace, use the `server_connect` tool with the server alias instead.",
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

		// 시스템에 저장된 인증 정보(ID/PW) 자동 연동
		finalCommand := command
		execOpts := workspace.ExecOptions{
			Shell:           "bash",
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
		isRemote := tool.IsRemoteWorkspace(ctx, targetAlias)
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
			)
			if err != nil {
				events <- tool.NewErrorEvent(err)
				return
			}
			syncActiveTargetFromLane(ctx, targetAlias)
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
			)
			if err != nil {
				events <- tool.NewErrorEvent(err)
				return
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
				events <- tool.NewErrorEvent(fmt.Errorf("bash shell not found"))
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
		)
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
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
	shellAutoBackgroundAfter = 3 * time.Second
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
) (utils.ExecResult, error) {
	shellPath, err := utils.FindSuitableShell()
	if err != nil {
		return utils.ExecResult{}, fmt.Errorf("find shell: %w", err)
	}
	if strings.TrimSpace(shellPath) == "" {
		return utils.ExecResult{}, fmt.Errorf("bash shell not found")
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
