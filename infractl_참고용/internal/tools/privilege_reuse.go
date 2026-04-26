package tools

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/privilege"
)

var (
	permissionFailureRegex = regexp.MustCompile(`(?i)(permission denied|operation not permitted|access denied|access is denied|not writable|current user lacks write access|read-only file system|eacces|cannot create|cannot open|failed to open)`)
	sudoCommandRegex       = regexp.MustCompile(`(^|[;&|]\s*)sudo(\s|$)`)
)

func isPermissionFailure(result executor.ExecResult, err error) bool {
	text := result.Stdout + "\n" + result.Stderr
	if err != nil {
		text += "\n" + err.Error()
	}
	return permissionFailureRegex.MatchString(text)
}

func executeWithAcquiredPrivilege(
	ctx context.Context,
	exec executor.Executor,
	command string,
	targetUser string,
) (executor.ExecResult, bool) {
	pse, ok := exec.(executor.PersistentSessionExecutor)
	if !ok || executor.CommandPlatform(exec) == executor.PlatformWindows {
		return executor.ExecResult{}, false
	}

	targetUser = privilege.NormalizeUser(targetUser)
	if si, ok := findAliveSessionByUser(ctx, pse, targetUser); ok {
		return executeInAcquiredSession(ctx, pse, si, command,
			fmt.Sprintf("[Privilege Reused: session %q already running as %s]", si.SessionID, targetUser))
	}

	rootSession, ok := findAliveSessionByUser(ctx, pse, "root")
	if !ok {
		return executor.ExecResult{}, false
	}

	if targetUser == "root" {
		return executeInAcquiredSession(ctx, pse, rootSession, command,
			fmt.Sprintf("[Privilege Reused: root session %q]", rootSession.SessionID))
	}

	runAsCommand, err := rootRunAsCommand(targetUser, command)
	if err != nil {
		slog.Debug("privilege reuse: build root run-as command failed", "user", targetUser, "err", err)
		return executor.ExecResult{}, false
	}
	return executeInAcquiredSession(ctx, pse, rootSession, runAsCommand,
		fmt.Sprintf("[Privilege Reused: root session %q -> %s]", rootSession.SessionID, targetUser))
}

func executePlainViaAcquiredRoot(
	ctx context.Context,
	exec executor.Executor,
	command string,
) (executor.ExecResult, bool) {
	pse, ok := exec.(executor.PersistentSessionExecutor)
	if !ok || executor.CommandPlatform(exec) == executor.PlatformWindows {
		return executor.ExecResult{}, false
	}
	rootSession, ok := findAliveSessionByUser(ctx, pse, "root")
	if !ok {
		return executor.ExecResult{}, false
	}
	return executeInAcquiredSession(ctx, pse, rootSession, command,
		fmt.Sprintf("[Privilege Reused: permission failure retried in root session %q]", rootSession.SessionID))
}

func executeInAcquiredSession(
	ctx context.Context,
	pse executor.PersistentSessionExecutor,
	session executor.SessionInfo,
	command string,
	note string,
) (executor.ExecResult, bool) {
	result, err := pse.SessionExecute(ctx, session.SessionID, command, timeoutFromContext(ctx, 30*time.Second), nil)
	if err != nil {
		slog.Debug("privilege reuse: session execution failed", "session", session.SessionID, "err", err)
		return executor.ExecResult{}, false
	}
	return shellRunToExecResult(result, note), true
}

func shellRunToExecResult(result executor.ShellRunResult, note string) executor.ExecResult {
	stdout := strings.TrimSpace(result.Stdout)
	note = strings.TrimSpace(note)
	if note != "" {
		if stdout != "" {
			stdout = note + "\n" + stdout
		} else {
			stdout = note
		}
	}
	return executor.ExecResult{
		Stdout:   stdout,
		ExitCode: result.ExitCode,
		Duration: result.Duration,
	}
}

func stripPrivilegeReuseNote(output string) string {
	output = strings.TrimPrefix(output, "\ufeff")
	if !strings.HasPrefix(output, "[Privilege Reused:") {
		return output
	}
	if idx := strings.IndexByte(output, '\n'); idx >= 0 {
		return output[idx+1:]
	}
	return ""
}

func findAliveSessionByUser(ctx context.Context, pse executor.PersistentSessionExecutor, user string) (executor.SessionInfo, bool) {
	sessions, err := pse.SessionList(ctx)
	if err != nil {
		return executor.SessionInfo{}, false
	}
	user = privilege.NormalizeUser(user)

	sort.Slice(sessions, func(i, j int) bool {
		iPreferred := strings.EqualFold(sessions[i].SessionID, user)
		jPreferred := strings.EqualFold(sessions[j].SessionID, user)
		if iPreferred != jPreferred {
			return iPreferred
		}
		return sessions[i].LastUsed.After(sessions[j].LastUsed)
	})

	for _, si := range sessions {
		if !si.Alive {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(si.CurrentUser), user) {
			return si, true
		}
	}
	return executor.SessionInfo{}, false
}

func rootRunAsCommand(user, command string) (string, error) {
	plan, err := privilege.BuildPlan(privilege.MethodSU, user, command)
	if err != nil {
		return "", err
	}
	return plan.InteractiveRun, nil
}

func timeoutFromContext(ctx context.Context, fallback time.Duration) time.Duration {
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining > time.Second {
			return remaining
		}
	}
	return fallback
}

func inferSuccessfulSudoUser(command string) (string, bool) {
	loc := sudoCommandRegex.FindStringIndex(command)
	if loc == nil {
		return "", false
	}

	segment := strings.TrimLeft(command[loc[0]:], ";&| \t")
	fields := strings.Fields(segment)
	if len(fields) == 0 || fields[0] != "sudo" {
		return "", false
	}

	for i := 1; i < len(fields); i++ {
		field := strings.TrimSpace(fields[i])
		if field == "--" {
			break
		}
		if field == "-u" || field == "--user" {
			if i+1 < len(fields) {
				return trimShellWord(fields[i+1]), true
			}
			break
		}
		if strings.HasPrefix(field, "-u") && len(field) > 2 {
			return trimShellWord(field[2:]), true
		}
		if strings.HasPrefix(field, "--user=") {
			return trimShellWord(strings.TrimPrefix(field, "--user=")), true
		}
		if !strings.HasPrefix(field, "-") {
			break
		}
	}
	return "root", true
}

func trimShellWord(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	if value == "" {
		return "root"
	}
	return value
}

// extractElevationTargetUser parses the target OS user from an elevation command.
// Used to route elevation to the correct independent session instead of mutating
// the current session's user context.
//
// Examples:
//   - "su - oracle"       → "oracle"
//   - "su -l oracle"      → "oracle"
//   - "sudo -i -u oracle" → "oracle"
//   - "sudo -i"           → "root"
//   - "sudo bash"         → "root"
func extractElevationTargetUser(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return "root"
	}

	switch fields[0] {
	case "su":
		// su [-] [-l] [user]
		// Skip flags (--, -, -l, -m, -p, -s shell) to find username
		for i := 1; i < len(fields); i++ {
			f := fields[i]
			if f == "--" {
				if i+1 < len(fields) {
					return trimShellWord(fields[i+1])
				}
				return "root"
			}
			// -s takes a shell argument — skip next token
			if f == "-s" || f == "--shell" {
				i++
				continue
			}
			// single dash alone is the "login" flag, not a user name
			if f == "-" {
				continue
			}
			if strings.HasPrefix(f, "-") {
				// combined short flags like -lm — no username here
				continue
			}
			// first non-flag token is the username
			return trimShellWord(f)
		}
		return "root"

	case "sudo":
		// sudo [-i|-s] [-u user] [command]
		for i := 1; i < len(fields); i++ {
			f := fields[i]
			if f == "-u" || f == "--user" {
				if i+1 < len(fields) {
					return trimShellWord(fields[i+1])
				}
				return "root"
			}
			if strings.HasPrefix(f, "-u") && len(f) > 2 {
				return trimShellWord(f[2:])
			}
			if strings.HasPrefix(f, "--user=") {
				return trimShellWord(strings.TrimPrefix(f, "--user="))
			}
		}
		return "root"

	case "exec":
		// exec sudo ... — recurse into remainder
		if len(fields) > 1 {
			return extractElevationTargetUser(strings.Join(fields[1:], " "))
		}
		return "root"

	default:
		return "root"
	}
}
