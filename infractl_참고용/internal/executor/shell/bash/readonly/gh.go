// Package readonly
// File: gh.go
// Description: gh CLI 읽기 전용 서브커맨드 화이트리스트
// Responsibility: GhReadOnlyCommands 매핑 제공

// Ported from: claude_cli/src/utils/shell/readOnlyCommandValidation.ts:GH_READ_ONLY_COMMANDS

package readonly

// GhReadOnlyCommands 는 안전한 gh CLI 서브커맨드 목록이다.
var GhReadOnlyCommands = map[string]CommandConfig{
	"gh pr list": {SafeFlags: map[string]FlagArgType{
		"--state": FlagArgString, "-s": FlagArgString,
		"--limit": FlagArgNumber, "-L": FlagArgNumber,
		"--label": FlagArgString, "-l": FlagArgString,
		"--assignee": FlagArgString, "-a": FlagArgString,
		"--author": FlagArgString, "-A": FlagArgString,
		"--base": FlagArgString, "-B": FlagArgString,
		"--head": FlagArgString, "-H": FlagArgString,
		"--draft": FlagArgNone, "-d": FlagArgNone,
		"--json": FlagArgString,
		"--jq": FlagArgString, "-q": FlagArgString,
		"--template": FlagArgString, "-t": FlagArgString,
		"--repo": FlagArgString, "-R": FlagArgString,
		"--search": FlagArgString,
	}},
	"gh pr view": {SafeFlags: map[string]FlagArgType{
		"--comments": FlagArgNone, "-c": FlagArgNone,
		"--json": FlagArgString, "--jq": FlagArgString, "-q": FlagArgString,
		"--template": FlagArgString, "-t": FlagArgString,
		"--repo": FlagArgString, "-R": FlagArgString,
	}},
	"gh pr checks": {SafeFlags: map[string]FlagArgType{
		"--repo": FlagArgString, "-R": FlagArgString,
		"--json": FlagArgString, "--jq": FlagArgString, "-q": FlagArgString,
	}},
	"gh issue list": {SafeFlags: map[string]FlagArgType{
		"--state": FlagArgString, "-s": FlagArgString,
		"--limit": FlagArgNumber, "-L": FlagArgNumber,
		"--label": FlagArgString, "-l": FlagArgString,
		"--assignee": FlagArgString, "-a": FlagArgString,
		"--author": FlagArgString, "-A": FlagArgString,
		"--mention": FlagArgString, "-m": FlagArgString,
		"--milestone": FlagArgString,
		"--json": FlagArgString,
		"--jq": FlagArgString, "-q": FlagArgString,
		"--template": FlagArgString, "-t": FlagArgString,
		"--repo": FlagArgString, "-R": FlagArgString,
		"--search": FlagArgString,
	}},
	"gh issue view": {SafeFlags: map[string]FlagArgType{
		"--comments": FlagArgNone, "-c": FlagArgNone,
		"--json": FlagArgString, "--jq": FlagArgString, "-q": FlagArgString,
		"--template": FlagArgString, "-t": FlagArgString,
		"--repo": FlagArgString, "-R": FlagArgString,
	}},
	"gh repo view": {SafeFlags: map[string]FlagArgType{
		"--json": FlagArgString, "--jq": FlagArgString, "-q": FlagArgString,
		"--template": FlagArgString, "-t": FlagArgString,
		"--repo": FlagArgString, "-R": FlagArgString,
	}},
	"gh run list": {SafeFlags: map[string]FlagArgType{
		"--workflow": FlagArgString, "-w": FlagArgString,
		"--limit": FlagArgNumber, "-L": FlagArgNumber,
		"--branch": FlagArgString, "-b": FlagArgString,
		"--actor": FlagArgString, "-u": FlagArgString,
		"--event": FlagArgString, "-e": FlagArgString,
		"--status": FlagArgString, "-s": FlagArgString,
		"--json": FlagArgString, "--jq": FlagArgString, "-q": FlagArgString,
		"--template": FlagArgString, "-t": FlagArgString,
		"--repo": FlagArgString, "-R": FlagArgString,
	}},
	"gh run view": {SafeFlags: map[string]FlagArgType{
		"--log": FlagArgNone, "--log-failed": FlagArgNone,
		"--exit-status": FlagArgNone,
		"--json": FlagArgString, "--jq": FlagArgString, "-q": FlagArgString,
		"--template": FlagArgString, "-t": FlagArgString,
		"--repo": FlagArgString, "-R": FlagArgString,
	}},
	"gh api": {SafeFlags: map[string]FlagArgType{
		"--method": FlagArgString, "-X": FlagArgString,
		"--field": FlagArgString, "-f": FlagArgString,
		"--raw-field": FlagArgString, "-F": FlagArgString,
		"--header": FlagArgString, "-H": FlagArgString,
		"--jq": FlagArgString, "-q": FlagArgString,
		"--template": FlagArgString, "-t": FlagArgString,
		"--include": FlagArgNone, "-i": FlagArgNone,
		"--paginate": FlagArgNone, "-p": FlagArgNone,
		"--silent": FlagArgNone,
	},
		// GET 메서드만 안전. POST/PATCH/DELETE 는 쓰기 작업
		IsDangerous: func(_ string, args []string) bool {
			for i, t := range args {
				if (t == "--method" || t == "-X") && i+1 < len(args) {
					m := args[i+1]
					return m != "GET" && m != "get"
				}
			}
			return false // 기본값 GET
		},
	},
}
