package tools

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/koreaf16/argus/internal/types"
	"github.com/koreaf16/argus/internal/utils/permissions"
	ipwsh "github.com/koreaf16/argus/internal/utils/powershell"
)

var shellWriteOperators = []*regexp.Regexp{
	regexp.MustCompile(`(^|[^\d])>>?`),
	regexp.MustCompile(`\|\s*tee\b`),
	regexp.MustCompile(`\btee\b`),
}

var sensitivePathHints = []string{
	".ssh",
	".aws",
	".gnupg",
	".bashrc",
	".zshrc",
	".gitconfig",
	".npmrc",
	"id_rsa",
	"id_ed25519",
	"credentials",
}

var installOrDownloadPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b(apt|apt-get|yum|dnf|pacman|brew)\b`),
	regexp.MustCompile(`\b(npm|pnpm|yarn|pip|pip3|gem|cargo)\s+(install|add)\b`),
	regexp.MustCompile(`\bgo\s+get\b`),
	regexp.MustCompile(`\b(curl|wget)\b`),
	regexp.MustCompile(`\bInvoke-WebRequest\b`),
}

var bashMutatingCommands = map[string]bool{
	"rm": true, "mv": true, "cp": true, "touch": true, "mkdir": true, "rmdir": true,
	"chmod": true, "chown": true, "chgrp": true, "sed": true, "perl": true, "git": true,
	"npm": true, "pnpm": true, "yarn": true, "pip": true, "pip3": true, "go": true, "cargo": true,
}

var bashReadOnlyCommands = map[string]bool{
	// 파일 조회
	"ls": true, "cat": true, "head": true, "tail": true, "pwd": true, "echo": true,
	"which": true, "whereis": true, "find": true, "grep": true, "rg": true, "ag": true,
	"wc": true, "stat": true, "diff": true, "git": true, "gh": true, "python": true, "python3": true,
	// 환경 조회 — export 는 파일시스템을 수정하지 않는 shell 내장 명령
	"export": true, "env": true, "printenv": true,
	// 프로세스 / 시스템 상태
	"ps": true, "df": true, "free": true, "uptime": true, "uname": true,
	"id": true, "whoami": true, "hostname": true, "date": true,
}

var gitReadOnlySubcommands = map[string]bool{
	"diff": true, "log": true, "show": true, "status": true, "branch": true, "tag": true,
	"remote": true, "ls-files": true, "grep": true, "blame": true, "describe": true,
	"rev-parse": true, "rev-list": true, "cat-file": true, "for-each-ref": true, "shortlog": true,
	"reflog": true, "ls-tree": true, "merge-base": true,
}

var ghReadOnlySubcommands = map[string]bool{
	"pr": true, "issue": true, "repo": true, "run": true, "workflow": true, "release": true,
	"auth": true, "search": true, "label": true,
}

func PermissionModeFromState(ctx Context) types.PermissionMode {
	if ctx.State == nil {
		return types.PermissionModeDefault
	}
	return ctx.State.GetPermissionMode()
}

func IsWriteModeAllowed(ctx Context) bool {
	switch PermissionModeFromState(ctx) {
	case types.PermissionModeBypassPermissions, types.PermissionModeDontAsk, types.PermissionModeAcceptEdits:
		return true
	default:
		return false
	}
}

func EvaluateShellCommandPermission(ctx Context, command string, shellKind string) types.PermissionResult {
	mode := PermissionModeFromState(ctx)
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return types.PermissionResult{Behavior: types.BehaviorAsk, Message: "empty command requires confirmation"}
	}

	if mode == types.PermissionModeBypassPermissions || mode == types.PermissionModeDontAsk {
		return types.PermissionResult{Behavior: types.BehaviorAllow}
	}

	if hasSensitiveWriteTarget(trimmed) {
		return types.PermissionResult{
			Behavior: types.BehaviorDeny,
			Message:  "command targets sensitive credential/config paths",
		}
	}

	if shouldAlwaysAsk(trimmed) {
		return types.PermissionResult{
			Behavior: types.BehaviorAsk,
			Message:  "command may install/download/modify environment",
		}
	}

	if isReadOnlyShellCommand(trimmed, shellKind) {
		return types.PermissionResult{Behavior: types.BehaviorAllow}
	}

	if mode == types.PermissionModeAcceptEdits && isFilesystemEditCommand(trimmed, shellKind) {
		return types.PermissionResult{Behavior: types.BehaviorAllow}
	}

	// 화이트리스트 불가 → LLM 분류기에 위임
	if ctx.ExecuteSubQuery != nil {
		classResult := permissions.ClassifyBashCommand(
			ctx.Context, trimmed, permissions.NewDenialTrackingState(),
			permissions.SubQueryFunc(ctx.ExecuteSubQuery),
		)
		if !classResult.Unavailable {
			switch classResult.Behavior {
			case types.ClassifierBehaviorAllow:
				return types.PermissionResult{
					Behavior: types.BehaviorAllow,
					DecisionReason: &types.PermissionDecisionReason{
						Type:   types.DecisionReasonClassifier,
						Reason: classResult.Reason,
					},
				}
			case types.ClassifierBehaviorDeny:
				return types.PermissionResult{
					Behavior: types.BehaviorDeny,
					Message:  classResult.Reason,
				}
			}
		}
	}

	return types.PermissionResult{
		Behavior: types.BehaviorAsk,
		Message:  "command may modify filesystem or system state",
	}
}

// IsReadOnlyShellCommand reports whether the given shell command is treated as
// read-only by the permission policy.
func IsReadOnlyShellCommand(command string, shellKind string) bool {
	return isReadOnlyShellCommand(command, shellKind)
}

func shouldAlwaysAsk(command string) bool {
	for _, re := range installOrDownloadPatterns {
		if re.MatchString(command) {
			return true
		}
	}
	return false
}

func hasSensitiveWriteTarget(command string) bool {
	if !looksMutating(command, "bash") && !looksMutating(command, "powershell") {
		return false
	}
	lower := strings.ToLower(command)
	for _, hint := range sensitivePathHints {
		if strings.Contains(lower, strings.ToLower(hint)) {
			return true
		}
	}
	return false
}

func isReadOnlyShellCommand(command string, shellKind string) bool {
	segments := splitCommandSegments(command)
	if len(segments) == 0 {
		return false
	}
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		if looksMutating(seg, shellKind) {
			return false
		}
		if !segmentIsReadOnly(seg, shellKind) {
			return false
		}
	}
	return true
}

func isFilesystemEditCommand(command string, shellKind string) bool {
	segments := splitCommandSegments(command)
	if len(segments) == 0 {
		return false
	}
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		if !looksMutating(seg, shellKind) {
			return false
		}
	}
	return true
}

func looksMutating(command string, shellKind string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))
	for _, re := range shellWriteOperators {
		if re.MatchString(lower) {
			return true
		}
	}

	cmd := firstToken(lower)
	if cmd == "" {
		return false
	}
	if shellKind == "powershell" {
		cmdlet := strings.TrimSpace(firstToken(command))
		if ipwsh.IsDangerousCmdlet(cmdlet) || ipwsh.IsPatternDangerous(command) {
			return true
		}
		if strings.HasPrefix(strings.ToLower(cmdlet), "set-") ||
			strings.HasPrefix(strings.ToLower(cmdlet), "new-") ||
			strings.HasPrefix(strings.ToLower(cmdlet), "remove-") ||
			strings.HasPrefix(strings.ToLower(cmdlet), "disable-") ||
			strings.HasPrefix(strings.ToLower(cmdlet), "stop-") {
			return true
		}
		return false
	}

	if bashMutatingCommands[cmd] {
		if cmd == "git" {
			sub := secondToken(lower)
			return sub != "" && !gitReadOnlySubcommands[sub]
		}
		if cmd == "sed" {
			return strings.Contains(lower, " -i") || strings.Contains(lower, "--in-place")
		}
		if cmd == "go" {
			sub := secondToken(lower)
			return sub == "get" || sub == "mod"
		}
		return true
	}
	return false
}

func segmentIsReadOnly(segment string, shellKind string) bool {
	if shellKind == "powershell" {
		cmd := strings.ToLower(firstToken(segment))
		if cmd == "" {
			return false
		}
		if strings.HasPrefix(cmd, "get-") || strings.HasPrefix(cmd, "select-") || strings.HasPrefix(cmd, "where-") {
			return true
		}
		switch cmd {
		case "echo", "write-output", "measure-object", "format-table", "format-list":
			return true
		default:
			return false
		}
	}

	cmd := firstToken(strings.ToLower(segment))
	if !bashReadOnlyCommands[cmd] {
		return false
	}
	if cmd == "git" {
		sub := secondToken(strings.ToLower(segment))
		return sub != "" && gitReadOnlySubcommands[sub]
	}
	if cmd == "gh" {
		sub := secondToken(strings.ToLower(segment))
		return sub != "" && ghReadOnlySubcommands[sub]
	}
	return true
}

func splitCommandSegments(command string) []string {
	replacer := strings.NewReplacer("&&", "\n", "||", "\n", ";", "\n", "|", "\n")
	raw := replacer.Replace(command)
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func firstToken(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func secondToken(s string) string {
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return ""
	}
	return fields[1]
}

func ExtractStringInput(input json.RawMessage, field string) string {
	var body map[string]any
	if err := json.Unmarshal(input, &body); err != nil {
		return ""
	}
	v, _ := body[field].(string)
	return strings.TrimSpace(v)
}
