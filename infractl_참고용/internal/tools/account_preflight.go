// Package tools
// File: account_preflight.go
// Description: tools 모듈의 기능 수행.
// Responsibility: tools 관련 로직 처리 및 관리.

package tools

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/privilege"
)

type MutationPathPreflight struct {
	Operation    string
	Paths        []string
	BecomeMethod string
	BecomeUser   string
}

type pathAccessAccount struct {
	CurrentUser   string
	EffectiveUser string
	Groups        map[string]struct{}
	Source        string
}

type pathAccessInfo struct {
	RequestedPath string
	CheckedPath   string
	Owner         string
	Group         string
	Mode          string
}

// ValidateMutationPathAccess blocks filesystem mutations that are very likely
// to fail due to owner/mode mismatch before the command can flood the session.
func ValidateMutationPathAccess(ctx context.Context, exec executor.Executor, opts MutationPathPreflight) error {
	if executor.CommandPlatform(exec) == executor.PlatformWindows {
		return nil
	}

	paths := normalizePreflightPaths(opts.Paths)
	if len(paths) == 0 {
		return nil
	}

	account, err := resolvePathAccessAccount(ctx, exec, opts.BecomeMethod, opts.BecomeUser)
	if err != nil {
		return fmt.Errorf("permission pre-check failed: %w", err)
	}
	if account.EffectiveUser == "" {
		return fmt.Errorf("permission pre-check failed: current account could not be determined; specify become_method and become_user")
	}
	if sameShellUser(account.EffectiveUser, "root") {
		return nil
	}

	for _, targetPath := range paths {
		info, err := statNearestWritablePath(ctx, exec, targetPath)
		if err != nil {
			return fmt.Errorf("permission pre-check failed: cannot inspect owner/mode for %q: %w; do not retry the same command without fixing account or path", targetPath, err)
		}
		if pathWritableByAccount(info, account) {
			continue
		}
		return fmt.Errorf("%s", formatPathAccessFailure(opts.Operation, info, account))
	}
	return nil
}

func ValidateCommandMutationPathAccess(ctx context.Context, exec executor.Executor, command, becomeMethod, becomeUser string) error {
	scan := ScanCommand(command)
	if !scan.IsDiskModifying || len(scan.TargetPaths) == 0 {
		return nil
	}
	return ValidateMutationPathAccess(ctx, exec, MutationPathPreflight{
		Operation:    scan.Description,
		Paths:        scan.TargetPaths,
		BecomeMethod: becomeMethod,
		BecomeUser:   becomeUser,
	})
}

func resolvePathAccessAccount(ctx context.Context, exec executor.Executor, becomeMethod, becomeUser string) (pathAccessAccount, error) {
	method := strings.TrimSpace(becomeMethod)
	user := strings.TrimSpace(becomeUser)
	account := pathAccessAccount{}

	if method != "" {
		user = privilege.NormalizeUser(user)
		account.EffectiveUser = user
		account.Source = "become"
	} else {
		current, err := detectCurrentShellUser(ctx, exec)
		if err != nil {
			return account, err
		}
		account.CurrentUser = strings.TrimSpace(current)
		account.EffectiveUser = account.CurrentUser
		account.Source = "current"
	}

	if account.EffectiveUser == "" {
		return account, nil
	}
	if sameShellUser(account.EffectiveUser, "root") {
		return account, nil
	}

	groups, err := detectUserGroups(ctx, exec, account.EffectiveUser, method == "")
	if err != nil {
		return account, err
	}
	account.Groups = groups
	return account, nil
}

func detectUserGroups(ctx context.Context, exec executor.Executor, user string, currentUser bool) (map[string]struct{}, error) {
	cmd := "id -Gn"
	if !currentUser {
		cmd = fmt.Sprintf("id -Gn %s", executor.QuotePOSIX(user))
	}
	result, err := exec.Execute(ctx, cmd)
	if err != nil || result.ExitCode != 0 {
		if currentUser {
			return nil, fmt.Errorf("could not determine groups for current user")
		}
		return nil, fmt.Errorf("target user %q does not exist or groups could not be determined", user)
	}

	groups := make(map[string]struct{})
	for _, group := range strings.Fields(result.Stdout) {
		groups[group] = struct{}{}
	}
	return groups, nil
}

func normalizePreflightPaths(paths []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(paths))
	for _, raw := range paths {
		path := trimShellWord(strings.TrimSpace(raw))
		if path == "" || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "$") {
			continue
		}
		if isAllowedCharacterDevicePath(path) {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}

func isAllowedCharacterDevicePath(path string) bool {
	switch strings.TrimRight(path, "/") {
	case "/dev/null", "/dev/zero", "/dev/stdin", "/dev/stdout", "/dev/stderr":
		return true
	default:
		return false
	}
}

func statNearestWritablePath(ctx context.Context, exec executor.Executor, requestedPath string) (pathAccessInfo, error) {
	quoted := executor.QuotePOSIX(requestedPath)
	script := fmt.Sprintf(`p=%s; d="$p"; while [ ! -e "$d" ] && [ "$d" != "/" ]; do d=$(dirname "$d"); done; [ -e "$d" ] || exit 2; stat -c '%%n	%%U	%%G	%%a' "$d" 2>/dev/null`, quoted)
	result, err := exec.Execute(ctx, script)
	if err != nil {
		return pathAccessInfo{}, err
	}
	if result.ExitCode != 0 {
		return pathAccessInfo{}, fmt.Errorf("stat command failed with exit code %d", result.ExitCode)
	}
	fields := strings.Split(strings.TrimSpace(result.Stdout), "\t")
	if len(fields) != 4 {
		return pathAccessInfo{}, fmt.Errorf("unexpected stat output %q", strings.TrimSpace(result.Stdout))
	}
	return pathAccessInfo{
		RequestedPath: requestedPath,
		CheckedPath:   strings.TrimSpace(fields[0]),
		Owner:         strings.TrimSpace(fields[1]),
		Group:         strings.TrimSpace(fields[2]),
		Mode:          strings.TrimSpace(fields[3]),
	}, nil
}

func pathWritableByAccount(info pathAccessInfo, account pathAccessAccount) bool {
	mode, err := strconv.ParseInt(info.Mode, 8, 64)
	if err != nil {
		return false
	}

	user := strings.TrimSpace(account.EffectiveUser)
	switch {
	case user == "":
		return false
	case sameShellUser(user, "root"):
		return true
	case sameShellUser(user, info.Owner):
		return mode&0200 != 0
	case groupContains(account.Groups, info.Group):
		return mode&0020 != 0
	default:
		return mode&0002 != 0
	}
}

func groupContains(groups map[string]struct{}, group string) bool {
	if group == "" {
		return false
	}
	_, ok := groups[group]
	return ok
}

func sameShellUser(u1, u2 string) bool {
	return strings.TrimSpace(strings.ToLower(u1)) == strings.TrimSpace(strings.ToLower(u2))
}

func detectCurrentShellUser(ctx context.Context, exec executor.Executor) (string, error) {
	res, err := exec.Execute(ctx, "whoami")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Stdout), nil
}

func formatPathAccessFailure(operation string, info pathAccessInfo, account pathAccessAccount) string {
	if strings.TrimSpace(operation) == "" {
		operation = "filesystem mutation"
	}
	retryUser := strings.TrimSpace(info.Owner)
	if retryUser == "" {
		retryUser = "root"
	}
	return fmt.Sprintf(
		"permission pre-check failed: %s target %q resolves to %q owned by %s:%s mode=%s; effective user %q lacks write access. Do not retry the same command. Retry with become_method=\"sudo\", become_user=\"%s\" or choose a writable path",
		operation,
		info.RequestedPath,
		info.CheckedPath,
		info.Owner,
		info.Group,
		info.Mode,
		account.EffectiveUser,
		retryUser,
	)
}
