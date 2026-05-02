package bash

import (
	"os"
	"strings"

	"github.com/koreaf16/argus/internal/services/workspace"
	"github.com/koreaf16/argus/internal/services/workspace/channel"
	"github.com/koreaf16/argus/internal/services/workspace/lane"
	tool "github.com/koreaf16/argus/internal/tools"
)

// channelBackboneActive mirrors workspace.useChannelBackbone so the bash tool
// can reason about whether the channel layer is the authoritative router
// without importing workspace's private helper.
func channelBackboneActive() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ARGUS_CHANNEL")))
	switch v {
	case "0", "false", "off", "no":
		return false
	default:
		return true
	}
}

// channelRoutingPrivilege returns the privilege key the multi-channel backbone
// will use for the upcoming command, in the form "alias|<priv>". Local or
// non-Unix targets return "local"; remote targets reflect the post-RouteFor
// decision so aidebug traces show the exact lane that received the command.
// When the elevation policy rejects the command, returns "rejected:<code>".
func channelRoutingPrivilege(ctx tool.Context, alias, command string, isRemote bool, platform string) string {
	if !isRemote || platform != workspace.PlatformUnix {
		return "local"
	}
	if ctx.Workspace == nil {
		return "unknown"
	}
	loginUser := ""
	hostAlias := alias
	var policy channel.ElevationPolicy
	if entry, ok := ctx.Workspace.Registry().Get(alias); ok {
		if entry.Kind == workspace.ServerKindAccount {
			hostAlias = entry.ParentAlias
			if parent, parentOK := ctx.Workspace.Registry().Get(entry.ParentAlias); parentOK {
				loginUser = parent.User
				policy = channel.ElevationPolicy{
					Allowed:     parent.Elevation.Allowed,
					Mode:        parent.Elevation.Mode,
					TargetUsers: parent.Elevation.TargetUsers,
				}
			}
		} else {
			loginUser = entry.User
			policy = channel.ElevationPolicy{
				Allowed:     entry.Elevation.Allowed,
				Mode:        entry.Elevation.Mode,
				TargetUsers: entry.Elevation.TargetUsers,
			}
		}
	}
	stack := lane.AccountStack{}
	if loginUser != "" {
		stack = lane.AccountStack{loginUser}
	}
	if target, err := ctx.Workspace.ResolveExecutionTarget(alias); err == nil && target.IsAccount {
		stack = lane.AccountStack{loginUser, target.TargetUser}
		if tr := lane.ParseAccountTransition(command); tr.Kind == lane.AccountTransitionEnter || tr.Kind == lane.AccountTransitionInline {
			if strings.EqualFold(strings.TrimSpace(tr.User), "root") {
				stack = lane.AccountStack{loginUser}
			}
		}
	}
	if cm := ctx.Workspace.ChannelManager(); cm != nil {
		for _, snap := range cm.Snapshots() {
			if snap.Key.Alias != hostAlias {
				continue
			}
			if snap.Key.Purpose != channel.PurposeExec {
				continue
			}
			if len(snap.AccountStack) > 0 && alias == hostAlias {
				stack = snap.AccountStack
				break
			}
		}
	}
	decision := channel.RouteFor(hostAlias, stack, loginUser, command, policy, ctx.Workspace)
	if decision.Rejected != "" {
		return "rejected:" + decision.RejectCode
	}
	return string(decision.Privilege)
}

// channelRoutingTransition returns the transition kind ("none", "enter",
// "exit", "inline") that RouteFor would emit for the command.
func channelRoutingTransition(command string) string {
	switch lane.ParseAccountTransition(command).Kind {
	case lane.AccountTransitionEnter:
		return "enter"
	case lane.AccountTransitionExit:
		return "exit"
	case lane.AccountTransitionInline:
		return "inline"
	default:
		return "none"
	}
}

// channelRoutingRejectCode returns the rejection code from the elevation policy
// for the given command, or "" if the command is not rejected.
func channelRoutingRejectCode(ctx tool.Context, alias, command string, isRemote bool, platform string) string {
	priv := channelRoutingPrivilege(ctx, alias, command, isRemote, platform)
	if code, ok := strings.CutPrefix(priv, "rejected:"); ok {
		return code
	}
	return ""
}
