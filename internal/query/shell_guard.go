package query

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/services/workspace"
	"github.com/koreaf16/argus/internal/services/workspace/channel"
	"github.com/koreaf16/argus/internal/services/workspace/lane"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
	"github.com/koreaf16/argus/internal/utils/permissions"
)

type shellGuardPayload struct {
	Decision      string `json:"decision"`
	Risk          string `json:"risk"`
	ChannelAction string `json:"channel_action"`
	ChannelKey    string `json:"channel_key"`
	Server        string `json:"server"`
	EffectiveUser string `json:"effective_user"`
	ExecLabel     string `json:"exec_label"`
	// Shell records the effective shell only when it differs from the tool name.
	// For example, the bash tool on a Windows local workspace runs under
	// PowerShell, so this field lets the UI surface "(powershell)" alongside
	// the tool name. Empty when the tool name and effective shell match.
	Shell  string `json:"shell,omitempty"`
	Reason string `json:"reason,omitempty"`
}

var shellControlRE = regexp.MustCompile(`[;&|()]`)

func (e *Engine) prepareShellToolCall(ctx context.Context, call llm.ToolUseStart, toolCtx tool.Context, turnIndex int) (llm.ToolUseStart, bool, string) {
	if !isShellTool(call.Name) {
		return call, false, ""
	}

	args := map[string]any{}
	if err := json.Unmarshal(call.Input, &args); err != nil {
		return shellGuardDeniedCall(call, "blocked", "high", "invalid shell tool input"), true, "invalid shell tool input"
	}

	shellKind := strings.ToLower(strings.TrimSpace(call.Name))
	command, _ := args["command"].(string)
	command = strings.TrimSpace(command)
	if command == "" {
		return shellGuardDeniedCall(call, "blocked", "high", "empty command"), true, "empty command"
	}

	asUser, _ := args["as_user"].(string)
	asUser = strings.TrimSpace(asUser)

	normalized, normalizedUser, legacy, denyReason := normalizeInlinePrivilege(command, asUser, shellKind)
	if denyReason != "" {
		args["command"] = displayBlockedCommand(command)
		args["_shell_guard"] = shellGuardPayload{
			Decision: "denied",
			Risk:     "high",
			Reason:   denyReason,
		}
		call.Input = mustMarshalRaw(args)
		return call, true, denyReason
	}
	command = normalized
	asUser = normalizedUser
	args["command"] = command
	if asUser != "" {
		args["as_user"] = asUser
	}

	server, effectiveUser, execLabel, channelKey, action := resolveShellGuardContext(toolCtx, shellKind, args, asUser)
	risk, reason := e.classifyShellGuardRisk(ctx, shellKind, command, turnIndex)
	if legacy && reason == "" {
		reason = "inline privilege syntax normalized to as_user"
	}

	if shellKind == "powershell" && asUser != "" {
		msg := "PowerShell account channels are not supported yet; select a registered Windows account channel before using as_user"
		args["command"] = displayBlockedCommand(command)
		args["_shell_guard"] = shellGuardPayload{
			Decision:      "denied",
			Risk:          "high",
			ChannelAction: "deny",
			ChannelKey:    channelKey,
			Server:        server,
			EffectiveUser: effectiveUser,
			ExecLabel:     execLabel,
			Reason:        msg,
		}
		call.Input = mustMarshalRaw(args)
		return call, true, msg
	}

	args["_shell_guard"] = shellGuardPayload{
		Decision:      "allow",
		Risk:          risk,
		ChannelAction: action,
		ChannelKey:    channelKey,
		Server:        server,
		EffectiveUser: effectiveUser,
		ExecLabel:     execLabel,
		Shell:         effectiveShellForGuard(toolCtx, shellKind, args),
		Reason:        reason,
	}
	call.Input = mustMarshalRaw(args)
	return call, false, ""
}

// effectiveShellForGuard returns the shell that the tool will actually use,
// but only when it differs from the tool's name. The bash tool on a Windows
// local workspace transparently falls back to PowerShell (see
// internal/tools/bash.resolveBashLocalShell), and surfacing that in the guard
// payload lets the UI render "(powershell)" alongside the tool label.
// Returns "" when the effective shell matches the tool name (no surprise).
func effectiveShellForGuard(toolCtx tool.Context, shellKind string, args map[string]any) string {
	if shellKind != "bash" {
		return ""
	}
	rawServer, _ := args["server"].(string)
	targetInfo, err := tool.ResolveShellTargetInfo(toolCtx, rawServer, false)
	if err != nil {
		return ""
	}
	alias := tool.ResolveWorkspaceAlias(toolCtx, rawServer)
	isRemote := tool.IsRemoteWorkspace(toolCtx, alias)
	if !isRemote && targetInfo.Platform == workspace.PlatformWindows {
		return "powershell"
	}
	return ""
}

func (e *Engine) classifyShellGuardRisk(ctx context.Context, shellKind, command string, turnIndex int) (string, string) {
	risk := "medium"
	if tool.IsReadOnlyShellCommand(command, shellKind) {
		risk = "low"
	}

	e.mu.RLock()
	hasLLM := e.llm != nil
	e.mu.RUnlock()
	if !hasLLM {
		return risk, ""
	}

	subQuery, search, fetch := e.classificationFunctions(ctx, turnIndex)
	classResult := permissions.ClassifyBashCommand(ctx, command, permissions.NewDenialTrackingState(), subQuery, search, fetch)
	if classResult.Unavailable {
		return risk, classResult.Reason
	}
	switch classResult.Behavior {
	case types.ClassifierBehaviorAllow:
		return "low", classResult.Reason
	case types.ClassifierBehaviorDeny:
		return "high", classResult.Reason
	default:
		return risk, classResult.Reason
	}
}

func resolveShellGuardContext(ctx tool.Context, shellKind string, args map[string]any, asUser string) (server, effectiveUser, execLabel, channelKey, action string) {
	rawServer, _ := args["server"].(string)
	rawRole, _ := args["role"].(string)
	rawChannel, _ := args["channel"].(string)
	roleCtx, err := tool.ResolveExecutionRole(ctx, rawServer, rawRole, rawChannel, shellKind)
	if err == nil && strings.TrimSpace(roleCtx.Server) != "" {
		rawServer = roleCtx.Server
	}
	alias := tool.ResolveWorkspaceAlias(ctx, rawServer)
	if strings.TrimSpace(alias) == "" {
		alias = workspace.LocalAlias
	}
	server = alias

	effectiveUser = strings.TrimSpace(asUser)
	if effectiveUser == "" && strings.TrimSpace(roleCtx.AsUser) != "" {
		effectiveUser = strings.TrimSpace(roleCtx.AsUser)
	}

	if ctx.Workspace == nil || alias == workspace.LocalAlias {
		if effectiveUser == "" {
			effectiveUser = firstNonEmptyString(os.Getenv("USER"), os.Getenv("USERNAME"), "local")
		}
		execLabel = alias + "/" + effectiveUser
		return server, effectiveUser, execLabel, alias + "|" + effectiveUser + "|exec", "local"
	}

	target, err := ctx.Workspace.ResolveExecutionTarget(alias)
	if err != nil {
		if effectiveUser == "" {
			effectiveUser = "unknown"
		}
		execLabel = alias + "/" + effectiveUser
		return server, effectiveUser, execLabel, alias + "|" + effectiveUser + "|exec", "select"
	}

	hostAlias := target.HostAlias
	loginUser := strings.TrimSpace(target.HostEntry.User)
	if effectiveUser == "" {
		if target.IsAccount {
			effectiveUser = strings.TrimSpace(target.TargetUser)
		}
		if effectiveUser == "" {
			effectiveUser = loginUser
		}
	}
	if effectiveUser == "" {
		effectiveUser = "login"
	}

	stack := lane.AccountStack{}
	if loginUser != "" {
		stack = append(stack, loginUser)
	}
	if effectiveUser != "" && loginUser != "" && !strings.EqualFold(effectiveUser, loginUser) {
		stack = append(stack, effectiveUser)
	} else if effectiveUser != "" && loginUser == "" {
		stack = append(stack, effectiveUser)
	}
	priv := channel.PrivilegeKeyFromStack(loginUser, stack)
	key := channel.ChannelKey{Alias: hostAlias, Privilege: priv, Purpose: channel.PurposeExec}
	channelKey = displayChannelKey(key, loginUser, effectiveUser)
	execLabel = alias + "/" + effectiveUser

	action = "create"
	for _, snap := range ctx.Workspace.ChannelManager().Snapshots() {
		if snap.Key.Alias == key.Alias && snap.Key.Privilege == key.Privilege && snap.Key.Purpose == key.Purpose {
			action = "reused"
			break
		}
	}
	return server, effectiveUser, execLabel, channelKey, action
}

func normalizeInlinePrivilege(command, asUser, shellKind string) (string, string, bool, string) {
	trimmed := strings.TrimSpace(command)
	if shellKind == "powershell" {
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "runas") || strings.Contains(lower, "-verb runas") {
			return trimmed, asUser, false, "inline privilege syntax is not allowed; use as_user/account channel"
		}
		return trimmed, asUser, false, ""
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return trimmed, asUser, false, ""
	}
	first := fields[0]
	if first != "sudo" && first != "su" {
		if containsInlinePrivilegeToken(trimmed) {
			return trimmed, asUser, false, "inline privilege syntax is ambiguous; use as_user"
		}
		return trimmed, asUser, false, ""
	}
	if asUser != "" {
		return trimmed, asUser, false, "privilege must be selected with as_user, not inline sudo/su"
	}
	if first == "su" {
		if extracted, extractedUser, ok := extractSuCommand(trimmed); ok {
			return extracted, extractedUser, true, ""
		}
		return trimmed, asUser, false, "su shell transitions are not allowed in command; use as_user"
	}

	target := "root"
	i := 1
	for i < len(fields) {
		f := fields[i]
		switch {
		case f == "-u" || f == "--user":
			if i+1 >= len(fields) {
				return trimmed, asUser, false, "sudo target user is missing"
			}
			target = fields[i+1]
			i += 2
		case strings.HasPrefix(f, "-u") && len(f) > 2:
			target = strings.TrimPrefix(f, "-u")
			i++
		case strings.HasPrefix(f, "--user="):
			target = strings.TrimPrefix(f, "--user=")
			i++
		case strings.HasPrefix(f, "-"):
			i++
		default:
			body := strings.TrimSpace(strings.Join(fields[i:], " "))
			if body == "" {
				return trimmed, asUser, false, "sudo command body is missing"
			}
			if shellControlRE.MatchString(body) {
				return trimmed, asUser, false, "mixed privilege command must be split before execution"
			}
			return body, target, true, ""
		}
	}
	return trimmed, asUser, false, "sudo command body is missing"
}

func containsInlinePrivilegeToken(command string) bool {
	fields := strings.Fields(command)
	for _, f := range fields {
		switch f {
		case "sudo", "su", "doas", "pkexec", "runuser":
			return true
		}
	}
	return false
}

func shellGuardDeniedCall(call llm.ToolUseStart, command, risk, reason string) llm.ToolUseStart {
	args := map[string]any{
		"command": command,
		"_shell_guard": shellGuardPayload{
			Decision: "denied",
			Risk:     risk,
			Reason:   reason,
		},
	}
	call.Input = mustMarshalRaw(args)
	return call
}

func displayBlockedCommand(command string) string {
	if strings.TrimSpace(command) == "" {
		return "blocked"
	}
	return "blocked"
}

func displayChannelKey(key channel.ChannelKey, loginUser, effectiveUser string) string {
	priv := string(key.Privilege)
	if priv == "" {
		priv = firstNonEmptyString(loginUser, effectiveUser, "login")
	}
	return fmt.Sprintf("%s|%s|%s", key.Alias, priv, key.Purpose)
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func mustMarshalRaw(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

// extractSuCommand extracts (command, user) from "su [-] <user> -c '<cmd>'" patterns.
// Returns (cmd, user, true) on match so the caller can open a proper account channel.
func extractSuCommand(cmd string) (string, string, bool) {
	re := regexp.MustCompile(`^su\s+(?:-\s+)?(\S+)\s+-c\s+['"](.+?)['"]$`)
	m := re.FindStringSubmatch(strings.TrimSpace(cmd))
	if len(m) == 3 {
		return m[2], m[1], true
	}
	return "", "", false
}
