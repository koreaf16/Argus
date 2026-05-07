package powershell

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/koreaf16/argus/internal/services/workspace"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/shellsignal"
	"github.com/koreaf16/argus/internal/types"
	"github.com/koreaf16/argus/internal/utils"
	"github.com/koreaf16/argus/internal/utils/shell"
)

type PowerShellTool struct {
	provider     shell.ShellProvider
	providerOnce sync.Once
	providerErr  error
}

func NewPowerShellTool() *PowerShellTool {
	return &PowerShellTool{}
}

func (t *PowerShellTool) ensureProvider() error {
	t.providerOnce.Do(func() {
		psPath, pathErr := shell.GetCachedPowerShellPath()
		if pathErr != nil {
			t.providerErr = fmt.Errorf("PowerShell을 찾을 수 없습니다: %w", pathErr)
			return
		}
		t.provider = shell.CreatePowerShellProvider(psPath)
	})
	return t.providerErr
}

func (t *PowerShellTool) Name() string {
	return "powershell"
}

func (t *PowerShellTool) IsVisible(ctx tool.Context) bool {
	targetInfo, err := tool.ResolveShellTargetInfo(ctx, "", false)
	if err != nil {
		return true
	}
	if targetInfo.Platform == workspace.PlatformUnknown {
		return runtime.GOOS == "windows" // 플랫폼 미확인 시 로컬 OS 기준 fallback
	}
	return targetInfo.Platform == workspace.PlatformWindows
}

func (t *PowerShellTool) Description(ctx tool.Context) string {
	base := "PowerShell 명령을 실행합니다. " +
		"중요: 쉘 확장을 방지하기 위해 비밀번호, URL 또는 '!', '&'와 같은 특수 문자가 포함된 모든 문자열에는 항상 작은따옴표(')를 사용하세요. "
	if tool.RequiresExplicitServerAlias(ctx) {
		aliases := tool.RegisteredWorkspaceAliases(ctx)
		base += "여러 워크스페이스가 등록되어 있습니다 (" + strings.Join(aliases, ", ") + "). `server` 파라미터가 필수입니다. "
	}
	base += "\n\n멀티 채널 및 격리:\n" +
		"- 이 도구는 멀티 채널 SSH 백본을 통해 명령을 라우팅합니다. 명령은 (server, role, channel) 트리플렛에 의해 격리됩니다. 환경이나 현재 디렉토리를 공유하지 않고 작업을 병렬로 실행하려면 다른 역할/채널을 사용하세요."
	base += "\n\nShell Guard rule: select the execution account with `as_user`/role/channel when supported; do not use inline runas/elevation syntax in `command`."
	return base
}

func (t *PowerShellTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "실행할 PowerShell 명령",
			},
			"timeout_ms": map[string]any{
				"type":        "integer",
				"description": "선택적인 타임아웃(밀리초)",
			},
			"workdir": map[string]any{
				"type":        "string",
				"description": "선택적인 작업 디렉토리 (허용된 루트 내부에 있어야 함)",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "명령에 대한 선택적인 짧은 요약",
			},
			"server": map[string]any{
				"type":        "string",
				"description": "선택적인 워크스페이스 별칭. 기본값은 활성 워크스페이스입니다.",
			},
			"role": map[string]any{
				"type":        "string",
				"description": "선택적인 워크플로우 역할 (예: source_db, target_app). 세션 상태(PTY/CWD/ENV)를 격리하는 데 사용됩니다.",
			},
			"channel": map[string]any{
				"type":        "string",
				"description": "선택적인 워크플로우 채널 (source, target, transfer, verify). 세션 상태를 격리하는 데 사용됩니다.",
			},
			"as_user": map[string]any{
				"type":        "string",
				"description": "Optional execution account. Shell Guard validates the registered account channel before execution.",
			},
			"password": map[string]any{
				"type":        "string",
				"description": "원격 서버 인증을 위한 선택적인 SSH 비밀번호.",
			},
			"background": map[string]any{
				"type":        "boolean",
				"description": "true면 즉시 백그라운드 잡으로 시작하고 background_task_id를 반환합니다(자동 대기 없음). false면 포그라운드를 유지하며 자동 백그라운드 전환도 비활성화됩니다. 생략하면 오래 걸리는 명령의 경우 몇 초 후 자동으로 백그라운드로 전환됩니다.",
			},
			"stdin": map[string]any{
				"type":        "string",
				"description": "선택 사항: 명령 시작 시 stdin으로 한 번에 전달할 페이로드. 큰 텍스트(JSON, 패치, 임포트 데이터 등)는 command 인라인 대신 이 필드를 사용하세요. command는 `$input | ...`처럼 stdin을 읽는 파이프라인으로 작성합니다.",
			},
		},
		"required": []string{"command"},
	}
}

func (t *PowerShellTool) IsReadOnly() bool {
	return false
}

// IsConcurrencySafe allows orchestration to run independent read-only shell
// commands in parallel while keeping mutating commands serialized.
func (t *PowerShellTool) IsConcurrencySafe(input json.RawMessage) bool {
	command := tool.ExtractStringInput(input, "command")
	return tool.IsReadOnlyShellCommand(command, "powershell")
}

func (t *PowerShellTool) MaxResultSizeChars() int {
	return 100000
}

func (t *PowerShellTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	events := make(chan tool.ToolEvent, 2)

	var req struct {
		Command    string `json:"command"`
		TimeoutMS  int    `json:"timeout_ms"`
		WorkDir    string `json:"workdir"`
		Server     string `json:"server"`
		Role       string `json:"role"`
		Channel    string `json:"channel"`
		AsUser     string `json:"as_user"`
		Password   string `json:"password"`
		Background *bool  `json:"background"`
		Stdin      string `json:"stdin"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	go func() {
		defer close(events)
		command := strings.TrimSpace(req.Command)
		if command == "" {
			events <- tool.NewErrorEvent(fmt.Errorf("명령은 비어 있을 수 없습니다"))
			return
		}

		if err := tool.ValidateTextOnlyCapable("powershell", command); err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		if err := tool.ValidateCommandIntegrity("powershell", command); err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		roleCtx, err := tool.ResolveExecutionRole(ctx, req.Server, req.Role, req.Channel, "powershell")
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		req.Server = roleCtx.Server
		if err := tool.ValidateRoleMutation(roleCtx, "powershell", command, false); err != nil {
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
		if err := tool.ValidateShellCompatibility("powershell", targetInfo); err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}

		// 시스템에 저장된 인증 정보(ID/PW) 자동 연동
		if ctx.Workspace != nil && tool.IsRemoteWorkspace(ctx, targetAlias) {
			if req.Password == "" {
				req.Password = ctx.Workspace.GetPassword(targetAlias, "ssh")
			}
			if req.Password != "" {
				ctx.Workspace.SetPassword(targetAlias, "ssh", req.Password)
			}
		}

		if tool.IsRemoteWorkspace(ctx, targetAlias) {
			remoteCommand := buildRemotePowerShellCommand(command, req.WorkDir)
			forceBackground := req.Background != nil && *req.Background
			allowAutoBackground := req.Background == nil
			result, execErr := executeRemotePowerShellCommand(
				ctx,
				targetAlias,
				remoteCommand,
				req.TimeoutMS,
				events,
				forceBackground,
				allowAutoBackground,
				req.Stdin,
			)
			if execErr != nil {
				events <- tool.NewErrorEvent(execErr)
				return
			}
			resJSON, _ := json.Marshal(result)
			events <- tool.NewOutputEvent(string(resJSON))
			events <- tool.NewDoneEvent()
			return
		}

		if err := t.ensureProvider(); err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}

		workDir, err := tool.ResolveWorkingDirectory(ctx, req.WorkDir)
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
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

		resJSON, _ := json.Marshal(result)
		events <- tool.NewOutputEvent(string(resJSON))
		events <- tool.NewDoneEvent()
	}()

	return events, nil
}

const (
	autoBackgroundAfter = 3 * time.Second
	maxShellTailChars   = 128 * 1024
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
	psPath, err := shell.GetCachedPowerShellPath()
	if err != nil {
		return utils.ExecResult{}, err
	}

	timeout := clampPowerShellTimeout(timeoutMS)

	runCtx := ctx.Context
	runTimeout := timeout
	if ctx.ShellJobs != nil {
		runCtx = context.Background()
		runTimeout = 0
	}

	cmd := utils.RunCommand(
		runCtx,
		psPath,
		[]string{"-NoProfile", "-NonInteractive", "-Command", command},
		nil,
		workDir,
		runTimeout,
		utils.SupportsPTY(),
	)

	inputChan := make(chan string, 16)
	events <- tool.ToolEvent{
		Kind:          tool.ToolEventChunk,
		InputResponse: inputChan,
	}

	if stdinPayload != "" && cmd.Stdin != nil {
		go func(payload string) {
			_, _ = cmd.Stdin.Write([]byte(payload))
		}(stdinPayload)
	}

	var outputBuffer string
	streamCh := cmd.Stream
	resultCh := cmd.Result

	if forceBackground {
		if result, ok := startLocalBackgroundJob(ctx, cmd, targetAlias, command, outputBuffer, true); ok {
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
		autoBgT = time.NewTimer(autoBackgroundAfter)
		autoBgCh = autoBgT.C
		defer autoBgT.Stop()
	}

	for {
		select {
		case <-ctx.Context.Done():
			cmd.Kill()
			return utils.ExecResult{Interrupted: true, Code: 1}, ctx.Context.Err()
		case input := <-inputChan:
			if shellsignal.IsBackgroundRequest(input) {
				if result, ok := startLocalBackgroundJob(ctx, cmd, targetAlias, command, outputBuffer, true); ok {
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
				continue
			}
			outputBuffer = appendShellTail(outputBuffer, chunk, maxShellTailChars)
			events <- tool.NewChunkEvent(chunk)
		case res := <-resultCh:
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
				return result, nil
			}
		case <-timeoutCh:
			cmd.Kill()
			return utils.ExecResult{Code: 1, Stderr: "command timed out"}, fmt.Errorf("timeout")
		}
	}
}

func executeRemotePowerShellCommand(
	ctx tool.Context,
	targetAlias string,
	command string,
	timeoutMS int,
	events chan<- tool.ToolEvent,
	forceBackground bool,
	allowAutoBackground bool,
	stdinPayload string,
) (utils.ExecResult, error) {
	if ctx.Workspace == nil {
		return utils.ExecResult{}, fmt.Errorf("워크스페이스 관리자를 사용할 수 없습니다")
	}

	timeout := clampPowerShellTimeout(timeoutMS)

	runCtx := ctx.Context
	if ctx.ShellJobs != nil {
		runCtx = context.Background()
	}

	handle, err := ctx.Workspace.StartExecWithOptions(
		runCtx,
		targetAlias,
		command,
		workspace.ExecOptions{Shell: "powershell"},
		func(c context.Context, sys, user string) (string, error) {
			return ctx.ExecuteSubQuery(c, sys, user)
		},
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
		autoBgT = time.NewTimer(autoBackgroundAfter)
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
				return utils.ExecResult{Code: 1, Stderr: "원격 명령이 예기치 않게 종료되었습니다"}, fmt.Errorf("원격 명령이 예기치 않게 종료되었습니다")
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
			return utils.ExecResult{Code: 1, Stderr: "명령 시간이 초과되었습니다"}, fmt.Errorf("타임아웃")
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
	snap := ctx.ShellJobs.StartJob("powershell", targetAlias, command, writeFn, killFn, initialOutput)
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
	snap := ctx.ShellJobs.StartJob("powershell", targetAlias, command, writeFn, killFn, initialOutput)
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

func clampPowerShellTimeout(timeoutMS int) time.Duration {
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

func (t *PowerShellTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	rawServer := tool.ExtractStringInput(input, "server")
	rawRole := tool.ExtractStringInput(input, "role")
	rawChannel := tool.ExtractStringInput(input, "channel")
	roleCtx, err := tool.ResolveExecutionRole(ctx, rawServer, rawRole, rawChannel, "powershell")
	if err != nil {
		return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: err.Error()}, nil
	}
	rawServer = roleCtx.Server
	targetInfo, err := tool.ResolveShellTargetInfo(ctx, rawServer, false)
	if err != nil {
		return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: err.Error()}, nil
	}
	if targetInfo.Platform != workspace.PlatformUnknown {
		if err := tool.ValidateShellCompatibility("powershell", targetInfo); err != nil {
			return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: err.Error()}, nil
		}
	}
	command := tool.ExtractStringInput(input, "command")
	if err := tool.ValidateRoleMutation(roleCtx, "powershell", command, false); err != nil {
		return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: err.Error()}, nil
	}
	return tool.EvaluateShellCommandPermission(ctx, command, "powershell"), nil
}

func buildRemotePowerShellCommand(command, workDir string) string {
	trimmedDir := strings.TrimSpace(workDir)
	if trimmedDir == "" {
		return command
	}
	return fmt.Sprintf("Set-Location -LiteralPath %s; %s", quotePowerShellLiteral(trimmedDir), command)
}

func quotePowerShellLiteral(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "''") + "'"
}
