package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/koreaf16/argus/internal/services/workspace/channel"
	"github.com/koreaf16/argus/internal/services/workspace/lane"
)

// ElevationRejectedError is returned by startExecViaChannel when the server's
// elevation policy rejects a sudo/su command. Callers (StartExecWithOptions)
// must NOT fall through to legacy paths — the rejection is authoritative.
type ElevationRejectedError struct {
	Code   string
	Reason string
}

func (e *ElevationRejectedError) Error() string {
	return "elevation policy [" + e.Code + "]: " + e.Reason
}

// firstNonEmpty returns the first non-blank string in values, or "" if all
// are blank. Promoted to package-level after manager_lane.go was retired.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// useChannelBackbone reports whether the multi-channel routing layer should
// handle remote bash workloads. Defaults to enabled. Set ARGUS_CHANNEL=0 to
// fall back to lane / per-command exec while diagnosing regressions.
func useChannelBackbone() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ARGUS_CHANNEL")))
	switch v {
	case "0", "false", "off", "no":
		return false
	default:
		return true
	}
}

// shouldRouteViaChannel mirrors shouldRouteViaLane but for the new channel
// backbone. We restrict routing to remote bash so the blast radius is minimal
// during the cutover.
func (m *Manager) shouldRouteViaChannel(alias string, opts ExecOptions) bool {
	if !useChannelBackbone() {
		return false
	}
	if !m.IsRemoteAlias(alias) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(opts.Shell)) {
	case "", "bash", "sh":
		return true
	default:
		return false
	}
}

// elevationLegacyMode reports whether ARGUS_ELEVATION_LEGACY=1 is set.
// When true, servers with no elevation policy fall back to reuse_login (any
// target) so existing workflows continue to work without re-registration.
func elevationLegacyMode() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ARGUS_ELEVATION_LEGACY")))
	return v == "1" || v == "true" || v == "yes"
}

// elevationPolicyFor reads the Elevation config for alias from the registry
// and converts it to a channel.ElevationPolicy. When ARGUS_ELEVATION_LEGACY=1
// is set and the server has no explicit elevation policy, it falls back to
// reuse_login (any target) so pre-Phase-2 setups keep working without
// re-registration.
func (m *Manager) elevationPolicyFor(alias string) channel.ElevationPolicy {
	entry, ok := m.reg.Get(alias)
	if !ok {
		return channel.ElevationPolicy{}
	}
	policy := channel.ElevationPolicy{
		Allowed:     entry.Elevation.Allowed,
		Mode:        entry.Elevation.Mode,
		TargetUsers: entry.Elevation.TargetUsers,
	}
	// Legacy fallback: treat missing policy as reuse_login when the env var is set.
	if !policy.Allowed && elevationLegacyMode() {
		policy.Allowed = true
		policy.Mode = "reuse_login"
	}
	return policy
}

// execViaChannel runs a single command synchronously through the channel
// manager. It uses RouteFor to pick the privilege lane that matches the
// command's effect on the account stack and forces routing to that channel.
func (m *Manager) execViaChannel(ctx context.Context, alias, command string, opts ExecOptions) (ExecResult, error) {
	cm := m.ChannelManager()

	target, err := m.ResolveExecutionTarget(alias)
	if err != nil {
		return ExecResult{}, err
	}
	hostAlias := target.HostAlias
	loginUser, currentStack := m.routeStartStack(target, command)
	decision := channel.RouteFor(hostAlias, currentStack, loginUser, command, m.elevationPolicyFor(hostAlias), m)

	// Elevation policy reject: surface the reason before any channel acquire.
	if decision.Rejected != "" {
		return ExecResult{}, &ElevationRejectedError{Code: decision.RejectCode, Reason: decision.Rejected}
	}

	acquireOpts := channel.AcquireOpts{
		Password:     opts.Password,
		RootPassword: opts.RootPassword,
		Role:         opts.Role,
		Channel:      opts.Channel,
	}

	switch decision.Transition.Kind {
	case lane.AccountTransitionEnter:
		// Acquire the parent (current) channel and run EnterAccount on it.
		// The exec channel re-keys itself in place to the post-push privilege
		// so subsequent acquires for that privilege resolve to this same shell.
		parent, err := m.acquireExecForStack(ctx, cm, hostAlias, loginUser, currentStack, acquireOpts, target.SwitchMethod, m.passwordForEnter(hostAlias, currentStack.Top(), target.SwitchMethod, opts, ""))
		if err != nil {
			return ExecResult{}, err
		}
		// Prefer policy-resolved password; fall back to caller-supplied opts.
		password := m.passwordForEnter(hostAlias, decision.Transition.User, decision.Transition.Method, opts, decision.SudoPassword)
		if err := parent.EnterAccount(ctx, decision.Transition.User, decision.Transition.Method, password); err != nil {
			return ExecResult{}, err
		}
		// Update active privilege so the next command lands on the new key.
		m.setActivePrivilege(alias, decision.Privilege)
		// Re-register the exec channel under the new privilege key.
		_ = cm.PromoteExec(ctx, hostAlias, parent)
		snap := parent.Snapshot()
		return ExecResult{
			Stdout: fmt.Sprintf("entered %s as %s (stack: %s)", alias, snap.AccountStack.Top(), snap.AccountStack),
			Code:   0,
			CWD:    snap.CWD,
			User:   snap.AccountStack.Top(),
		}, nil

	case lane.AccountTransitionExit:
		// Acquire the channel currently holding the exit-source privilege and
		// run ExitAccount on it.
		ch, err := m.acquireExecForStack(ctx, cm, hostAlias, loginUser, currentStack, acquireOpts, target.SwitchMethod, m.passwordForEnter(hostAlias, currentStack.Top(), target.SwitchMethod, opts, ""))
		if err != nil {
			return ExecResult{}, err
		}
		if err := ch.ExitAccount(ctx); err != nil {
			return ExecResult{}, err
		}
		afterPriv := channel.PrivilegeKeyFromStack(loginUser, decision.TargetStack)
		m.setActivePrivilege(alias, afterPriv)
		_ = cm.PromoteExec(ctx, hostAlias, ch)
		snap := ch.Snapshot()
		top := snap.AccountStack.Top()
		if top == "" {
			top = loginUser
		}
		return ExecResult{
			Stdout: fmt.Sprintf("exited; now %s (stack: %s)", top, snap.AccountStack),
			Code:   0,
			CWD:    snap.CWD,
			User:   top,
		}, nil
	}

	// Plain or inline command: route to the channel matching the requested
	// privilege. AcquireExec for non-default privilege requires the channel
	// to already exist; if not, we synthesize an Enter on the current stack
	// so the user sees a clean error rather than a misroute.
	targetCh, err := m.acquireExecForStack(ctx, cm, hostAlias, loginUser, decision.TargetStack, acquireOpts, target.SwitchMethod, m.passwordForEnter(hostAlias, decision.TargetStack.Top(), target.SwitchMethod, opts, decision.SudoPassword))
	if err != nil {
		return ExecResult{}, fmt.Errorf("channel route to %q: %w", decision.Privilege, err)
	}

	workingDir := firstNonEmpty(opts.WorkingDir, target.DefaultCWD)
	req := channel.ExecRequest{
		Command:      command,
		WorkingDir:   workingDir,
		SudoPassword: firstNonEmpty(decision.SudoPassword, opts.RootPassword),
		OnChunk:      opts.ChunkCallback,
	}
	out, err := targetCh.Exec(ctx, req)
	if err != nil {
		return ExecResult{}, err
	}
	m.setActivePrivilege(alias, decision.Privilege)
	return ExecResult{
		Stdout: out.Stdout,
		Stderr: out.Stderr,
		Code:   out.Code,
		CWD:    out.CWD,
		User:   out.User,
	}, nil
}

// startExecViaChannel mirrors execViaChannel for the streaming caller path.
// Account transitions are processed synchronously before the streaming
// goroutine starts so the result channel only carries the post-transition
// status banner.
func (m *Manager) startExecViaChannel(ctx context.Context, alias, command string, opts ExecOptions) (*ExecHandle, error) {
	cm := m.ChannelManager()
	target, err := m.ResolveExecutionTarget(alias)
	if err != nil {
		return nil, err
	}
	hostAlias := target.HostAlias
	loginUser, currentStack := m.routeStartStack(target, command)
	decision := channel.RouteFor(hostAlias, currentStack, loginUser, command, m.elevationPolicyFor(hostAlias), m)

	// Elevation policy reject: return an authoritative error. StartExecWithOptions
	// must not fall through to legacy paths for this error kind.
	if decision.Rejected != "" {
		return nil, &ElevationRejectedError{Code: decision.RejectCode, Reason: decision.Rejected}
	}

	acquireOpts := channel.AcquireOpts{
		Password:     opts.Password,
		RootPassword: opts.RootPassword,
		Role:         opts.Role,
		Channel:      opts.Channel,
	}

	streamCh := make(chan string, 64)
	resultCh := make(chan ExecResult, 1)
	cancelCtx, cancel := context.WithCancel(ctx)
	var killOnce sync.Once

	handle := &ExecHandle{
		Stream: streamCh,
		Result: resultCh,
		Write: func(string) error {
			return errors.New("channel: stdin writes are not supported on exec channels")
		},
		Kill: func() { killOnce.Do(cancel) },
	}

	switch decision.Transition.Kind {
	case lane.AccountTransitionEnter, lane.AccountTransitionExit:
		go func() {
			defer close(streamCh)
			defer close(resultCh)
			defer cancel()
			res, err := m.execViaChannel(cancelCtx, alias, command, opts)
			if err != nil {
				resultCh <- ExecResult{Code: 1, Stderr: err.Error()}
				return
			}
			resultCh <- res
		}()
		return handle, nil
	}

	targetCh, err := m.acquireExecForStack(ctx, cm, hostAlias, loginUser, decision.TargetStack, acquireOpts, target.SwitchMethod, m.passwordForEnter(hostAlias, decision.TargetStack.Top(), target.SwitchMethod, opts, decision.SudoPassword))
	if err != nil {
		return nil, err
	}

	workingDir := firstNonEmpty(opts.WorkingDir, target.DefaultCWD)
	req := channel.ExecRequest{
		Command:      command,
		WorkingDir:   workingDir,
		SudoPassword: firstNonEmpty(decision.SudoPassword, opts.RootPassword),
		OnChunk: func(chunk string) {
			if opts.ChunkCallback != nil {
				opts.ChunkCallback(chunk)
			}
			select {
			case streamCh <- chunk:
			default:
			}
		},
	}

	go func() {
		defer close(streamCh)
		defer close(resultCh)
		defer cancel()

		out, err := targetCh.Exec(cancelCtx, req)
		if err != nil {
			resultCh <- ExecResult{Code: 1, Stderr: err.Error()}
			return
		}
		m.setActivePrivilege(alias, decision.Privilege)
		resultCh <- ExecResult{
			Stdout: out.Stdout,
			Stderr: out.Stderr,
			Code:   out.Code,
			CWD:    out.CWD,
			User:   out.User,
		}
	}()
	return handle, nil
}

// convertChannelFileEntries adapts the channel package's FileEntry slice into
// the workspace.FileEntry shape expected by callers. The two structs carry
// the same fields; the conversion exists only to bridge the import boundary.
func convertChannelFileEntries(in []channel.FileEntry) []FileEntry {
	out := make([]FileEntry, 0, len(in))
	for _, e := range in {
		out = append(out, FileEntry{
			Name:    e.Name,
			Path:    e.Path,
			IsDir:   e.IsDir,
			Size:    e.Size,
			Mode:    e.Mode,
			ModTime: e.ModTime,
		})
	}
	return out
}

// convertChannelTunnel adapts channel.TunnelInfo to workspace.TunnelInfo for
// the same reason as convertChannelFileEntries.
func convertChannelTunnel(in channel.TunnelInfo) TunnelInfo {
	return TunnelInfo{
		ID:         in.ID,
		Alias:      in.Alias,
		LocalAddr:  in.LocalAddr,
		RemoteAddr: in.RemoteAddr,
		Active:     in.Active,
	}
}

func laneStackForTarget(target ExecutionTarget) lane.AccountStack {
	loginUser := strings.TrimSpace(target.HostEntry.User)
	if !target.IsAccount {
		if loginUser == "" {
			return lane.AccountStack{}
		}
		return lane.AccountStack{loginUser}
	}
	if loginUser == "" {
		return lane.AccountStack{target.TargetUser}
	}
	if strings.TrimSpace(target.TargetUser) == "" || strings.EqualFold(loginUser, target.TargetUser) {
		return lane.AccountStack{loginUser}
	}
	return lane.AccountStack{loginUser, target.TargetUser}
}

func (m *Manager) routeStartStack(target ExecutionTarget, command string) (string, lane.AccountStack) {
	loginUser := strings.TrimSpace(target.HostEntry.User)
	if !target.IsAccount {
		return m.snapshotStack(target.HostAlias)
	}
	tr := lane.ParseAccountTransition(command)
	if isRootTransition(tr) {
		if loginUser == "" {
			return loginUser, lane.AccountStack{}
		}
		return loginUser, lane.AccountStack{loginUser}
	}
	return loginUser, laneStackForTarget(target)
}

func isRootTransition(tr lane.AccountTransition) bool {
	if tr.Kind != lane.AccountTransitionEnter && tr.Kind != lane.AccountTransitionInline {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(tr.User), "root")
}

func (m *Manager) acquireExecForStack(ctx context.Context, cm channel.ChannelManager, hostAlias, loginUser string, stack lane.AccountStack, opts channel.AcquireOpts, method, password string) (channel.ExecCapable, error) {
	priv := channel.PrivilegeKeyFromStack(loginUser, stack)
	ch, err := cm.AcquireExec(ctx, hostAlias, priv, opts)
	if err == nil {
		return ch, nil
	}
	if priv == channel.PrivilegeDefault {
		return nil, err
	}

	cleaned := make(lane.AccountStack, 0, len(stack))
	for _, user := range stack {
		user = strings.TrimSpace(user)
		if user != "" {
			cleaned = append(cleaned, user)
		}
	}
	if len(cleaned) == 0 {
		return cm.AcquireExec(ctx, hostAlias, channel.PrivilegeDefault, opts)
	}
	if len(cleaned) == 1 {
		if loginUser == "" || strings.EqualFold(cleaned[0], loginUser) {
			return cm.AcquireExec(ctx, hostAlias, channel.PrivilegeDefault, opts)
		}
		parent, parentErr := cm.AcquireExec(ctx, hostAlias, channel.PrivilegeDefault, opts)
		if parentErr != nil {
			return nil, parentErr
		}
		if enterErr := parent.EnterAccount(ctx, cleaned[0], method, password); enterErr != nil {
			return nil, enterErr
		}
		_ = cm.PromoteExec(ctx, hostAlias, parent)
		return parent, nil
	}

	parentStack := cleaned[:len(cleaned)-1]
	targetUser := cleaned[len(cleaned)-1]
	parent, parentErr := m.acquireExecForStack(ctx, cm, hostAlias, loginUser, parentStack, opts, method, password)
	if parentErr != nil {
		return nil, parentErr
	}
	if enterErr := parent.EnterAccount(ctx, targetUser, method, password); enterErr != nil {
		return nil, enterErr
	}
	_ = cm.PromoteExec(ctx, hostAlias, parent)
	return parent, nil
}

func (m *Manager) passwordForEnter(hostAlias, targetUser, method string, opts ExecOptions, resolved string) string {
	targetUser = strings.TrimSpace(targetUser)
	method = normalizeSwitchMethod(method)
	if targetUser == "" {
		return firstNonEmpty(resolved, opts.RootPassword, opts.Password, m.GetLoginPassword(hostAlias))
	}
	switch method {
	case PrivilegeSudo:
		return firstNonEmpty(
			resolved,
			m.GetPasswordForTarget(hostAlias, "sudo", targetUser),
			opts.RootPassword,
			opts.Password,
			m.GetLoginPassword(hostAlias),
		)
	default:
		return firstNonEmpty(
			resolved,
			m.GetPasswordForTarget(hostAlias, "su", targetUser),
			opts.RootPassword,
			m.GetPasswordForTarget(hostAlias, "sudo", targetUser),
			opts.Password,
			m.GetLoginPassword(hostAlias),
		)
	}
}

// snapshotStack returns the login user and the most recent exec channel's
// account stack. If no exec channel has been opened yet, returns the login
// user from the registry (or empty if unknown) and an empty stack.
func (m *Manager) snapshotStack(alias string) (string, lane.AccountStack) {
	loginUser := ""
	if entry, ok := m.reg.Get(alias); ok {
		loginUser = entry.User
	}
	pool := m.channelPool()
	pkLogin, stack, ok := channel.LookupExecCurrentStack(pool, alias)
	if ok {
		if pkLogin != "" {
			loginUser = pkLogin
		}
		return loginUser, stack
	}
	// No channel snapshot yet: assume just the login user is active.
	if loginUser != "" {
		return loginUser, lane.AccountStack{loginUser}
	}
	return loginUser, lane.AccountStack{}
}
