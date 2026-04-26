package powershell

import (
	"context"
	"encoding/json"
	"fmt"
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
			t.providerErr = fmt.Errorf("powershell not found: %w", pathErr)
			return
		}
		t.provider = shell.CreatePowerShellProvider(psPath)
	})
	return t.providerErr
}

func (t *PowerShellTool) Name() string {
	return "powershell"
}

func (t *PowerShellTool) Description(ctx tool.Context) string {
	return "Execute PowerShell commands"
}

func (t *PowerShellTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The PowerShell command to execute",
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
			"password": map[string]any{
				"type":        "string",
				"description": "Optional SSH password for remote server authentication.",
			},
			"background": map[string]any{
				"type":        "boolean",
				"description": "Run as a background job. Omit to auto-background after a few seconds for long-running commands.",
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
		Password   string `json:"password"`
		Background *bool  `json:"background"`
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

		if ctx.Workspace != nil && strings.TrimSpace(req.Password) != "" {
			ctx.Workspace.SetPassword(targetAlias, "ssh", req.Password)
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
) (utils.ExecResult, error) {
	if ctx.Workspace == nil {
		return utils.ExecResult{}, fmt.Errorf("workspace manager is unavailable")
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
				return utils.ExecResult{Code: 1, Stderr: "remote command ended unexpectedly"}, fmt.Errorf("remote command ended unexpectedly")
			}
			return utils.ExecResult{Stdout: res.Stdout, Stderr: res.Stderr, Code: res.Code}, nil

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
