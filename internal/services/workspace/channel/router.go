package channel

import (
	"strings"

	"github.com/koreaf16/argus/internal/services/workspace/lane"
)

// RoutingDecision is what RouteFor produces. It tells the caller which
// privilege channel must execute the command and how the privilege channel's
// internal account stack should be mutated as a side-effect.
type RoutingDecision struct {
	// Privilege identifies the channel the command must run on. Callers MUST
	// route the command to the channel keyed by this Privilege; mismatched
	// routing is a bug, not a fallback.
	Privilege PrivilegeKey

	// Purpose is always PurposeExec for command routing. Kept for symmetry
	// with future interactive routing decisions.
	Purpose ChannelPurpose

	// Transition describes how the underlying lane's AccountStack changes
	// after the command runs. Enter pushes; Exit pops; Inline/None do not
	// mutate the stack but still execute the command on the resolved channel.
	Transition lane.AccountTransition

	// TargetStack is the stack that the resolved channel represents. For
	// Enter, this is currentStack.Push(transition.User). For Exit, this is
	// currentStack.Pop(). For Inline/None, this is currentStack unchanged.
	// The channel's PrivilegeKey must equal PrivilegeKeyFromStack(login, TargetStack).
	TargetStack lane.AccountStack

	// Rejected, when non-empty, means the elevation policy refused the command.
	// The channel layer must NOT acquire a channel; instead surface this string
	// as the tool error. Rejected is only set for Enter/Inline transitions.
	Rejected string
	// RejectCode is the machine-readable category of the rejection. Bash tool
	// uses it for the aidebug emit.
	RejectCode string

	// SudoPassword is the resolved password to inject when the channel enters
	// a privileged account. Set only when the elevation policy is satisfied.
	SudoPassword string
}

// RouteFor decides which privilege channel must execute the given command,
// based on the caller's current account stack. Inline transitions (e.g.
// `sudo -u postgres psql -c "..."`) DO NOT change the channel — the body runs
// in the current channel because it is one-shot and never opens a persistent
// shell.
//
// Enter transitions (`sudo -i`, `su -`, `sudo -i -u X`) DO change the channel:
// the command must execute on the lane that represents the post-push stack so
// that the resulting persistent shell lives there from the start.
//
// Exit transitions (`exit`, `logout`) execute on the current channel (the one
// being left) so its PTY observes the exit; the reported TargetStack reflects
// the stack AFTER pop so callers can update active state.
//
// loginUser is required to canonicalize the bottom of the stack — a stack
// containing only loginUser is equivalent to PrivilegeDefault.
//
// When policy is provided and the command is an escalation, EvaluateElevation
// is called before routing. If the verdict rejects the command, the returned
// RoutingDecision has Rejected set and callers must NOT acquire a channel.
// Pass a zero ElevationPolicy (Allowed=false) to enforce denial by default.
func RouteFor(alias string, currentStack lane.AccountStack, loginUser, command string,
	policy ElevationPolicy, creds CredentialResolver) RoutingDecision {
	tr := lane.ParseAccountTransition(command)
	cur := currentStack.Clone()

	switch tr.Kind {
	case lane.AccountTransitionEnter:
		target := cur.Push(tr.User)
		// Check elevation policy before committing to the new channel.
		hint := AccountTransitionHint{IsEscalation: true, TargetUser: tr.User}
		if verdict := EvaluateElevation(alias, policy, hint, creds); !verdict.Allow {
			return RoutingDecision{
				Privilege:   PrivilegeKeyFromStack(loginUser, target),
				Purpose:     PurposeExec,
				Transition:  tr,
				TargetStack: target,
				Rejected:    verdict.Reason,
				RejectCode:  verdict.Code,
			}
		} else {
			return RoutingDecision{
				Privilege:    PrivilegeKeyFromStack(loginUser, target),
				Purpose:      PurposeExec,
				Transition:   tr,
				TargetStack:  target,
				SudoPassword: verdict.Password,
			}
		}

	case lane.AccountTransitionExit:
		next, _ := cur.Pop()
		return RoutingDecision{
			Privilege:   PrivilegeKeyFromStack(loginUser, cur),
			Purpose:     PurposeExec,
			Transition:  tr,
			TargetStack: next,
		}

	case lane.AccountTransitionInline:
		hint := AccountTransitionHint{IsEscalation: true, TargetUser: tr.User}
		if verdict := EvaluateElevation(alias, policy, hint, creds); !verdict.Allow {
			return RoutingDecision{
				Privilege:   PrivilegeKeyFromStack(loginUser, cur),
				Purpose:     PurposeExec,
				Transition:  tr,
				TargetStack: cur,
				Rejected:    verdict.Reason,
				RejectCode:  verdict.Code,
			}
		} else {
			return RoutingDecision{
				Privilege:    PrivilegeKeyFromStack(loginUser, cur),
				Purpose:      PurposeExec,
				Transition:   tr,
				TargetStack:  cur,
				SudoPassword: verdict.Password,
			}
		}

	default: // None
		return RoutingDecision{
			Privilege:   PrivilegeKeyFromStack(loginUser, cur),
			Purpose:     PurposeExec,
			Transition:  tr,
			TargetStack: cur,
		}
	}
}

// IsPrivilegedCommand reports whether the command attempts a privilege
// transition that would require routing to a non-default channel. This is a
// cheap classifier the bash tool uses for telemetry; the authoritative
// decision is RouteFor.
func IsPrivilegedCommand(command string) bool {
	tr := lane.ParseAccountTransition(command)
	switch tr.Kind {
	case lane.AccountTransitionEnter, lane.AccountTransitionExit, lane.AccountTransitionInline:
		return true
	}
	return false
}

// DescribeDecision returns a short, human-readable summary of a decision for
// log lines like "channel_decision: enter root via sudo -> alias|alice>root|exec".
func DescribeDecision(d RoutingDecision, alias string) string {
	key := ChannelKey{Alias: alias, Privilege: d.Privilege, Purpose: d.Purpose}
	var verb string
	switch d.Transition.Kind {
	case lane.AccountTransitionEnter:
		verb = "enter " + d.Transition.User + " via " + d.Transition.Method
	case lane.AccountTransitionExit:
		verb = "exit"
	case lane.AccountTransitionInline:
		verb = "inline " + d.Transition.User + " via " + d.Transition.Method
	default:
		verb = "exec"
	}
	return verb + " -> " + key.String()
}

// privilegeStackEqualsLoginOnly returns true when stack is empty or contains
// only loginUser. Useful for tests and validation; not exported because the
// canonical check goes through PrivilegeKeyFromStack.
func privilegeStackEqualsLoginOnly(loginUser string, stack lane.AccountStack) bool {
	clean := make(lane.AccountStack, 0, len(stack))
	for _, u := range stack {
		if strings.TrimSpace(u) != "" {
			clean = append(clean, u)
		}
	}
	if len(clean) == 0 {
		return true
	}
	return len(clean) == 1 && clean[0] == loginUser
}
