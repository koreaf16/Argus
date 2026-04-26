// Package readonly
// File: git.go
// Description: git 읽기 전용 서브커맨드 화이트리스트
// Responsibility: GIT_READ_ONLY_COMMANDS 매핑 제공

// Ported from: claude_cli/src/utils/shell/readOnlyCommandValidation.ts:GIT_READ_ONLY_COMMANDS

package readonly

var gitStatFlags = map[string]FlagArgType{
	"--stat": FlagArgNone, "--numstat": FlagArgNone,
	"--shortstat": FlagArgNone, "--name-only": FlagArgNone, "--name-status": FlagArgNone,
}
var gitColorFlags = map[string]FlagArgType{"--color": FlagArgNone, "--no-color": FlagArgNone}
var gitPatchFlags = map[string]FlagArgType{
	"--patch": FlagArgNone, "-p": FlagArgNone, "--no-patch": FlagArgNone,
	"--no-ext-diff": FlagArgNone, "-s": FlagArgNone,
}
var gitLogDisplayFlags = map[string]FlagArgType{
	"--oneline": FlagArgNone, "--graph": FlagArgNone, "--decorate": FlagArgNone,
	"--no-decorate": FlagArgNone, "--date": FlagArgString, "--relative-date": FlagArgNone,
}
var gitCountFlags = map[string]FlagArgType{"--max-count": FlagArgNumber, "-n": FlagArgNumber}
var gitRefFlags = map[string]FlagArgType{
	"--all": FlagArgNone, "--branches": FlagArgNone, "--tags": FlagArgNone, "--remotes": FlagArgNone,
}
var gitDateFlags = map[string]FlagArgType{
	"--since": FlagArgString, "--after": FlagArgString, "--until": FlagArgString, "--before": FlagArgString,
}
var gitAuthorFlags = map[string]FlagArgType{
	"--author": FlagArgString, "--committer": FlagArgString, "--grep": FlagArgString,
}

func mergeFlags(maps ...map[string]FlagArgType) map[string]FlagArgType {
	out := make(map[string]FlagArgType)
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// GitReadOnlyCommands 는 안전한 git 서브커맨드 목록이다.
// 키는 "git <subcmd>" 형식이다.
var GitReadOnlyCommands = map[string]CommandConfig{
	"git diff": {SafeFlags: mergeFlags(gitStatFlags, gitColorFlags, map[string]FlagArgType{
		"--dirstat": FlagArgNone, "--summary": FlagArgNone, "--word-diff": FlagArgNone,
		"--cached": FlagArgNone, "--staged": FlagArgNone, "--check": FlagArgNone,
		"--exit-code": FlagArgNone, "--quiet": FlagArgNone, "--full-index": FlagArgNone,
		"--binary": FlagArgNone, "--abbrev": FlagArgNumber, "--find-renames": FlagArgNone,
		"--no-renames": FlagArgNone, "--diff-filter": FlagArgString,
		"--diff-algorithm": FlagArgString, "--histogram": FlagArgNone,
		"--patience": FlagArgNone, "--minimal": FlagArgNone,
		"--ignore-space-at-eol": FlagArgNone, "--ignore-space-change": FlagArgNone,
		"--ignore-all-space": FlagArgNone, "--ignore-blank-lines": FlagArgNone,
		"--relative": FlagArgString, "--no-index": FlagArgNone,
		"-p": FlagArgNone, "-u": FlagArgNone, "-M": FlagArgNone, "-C": FlagArgNone,
		"-B": FlagArgNone, "-D": FlagArgNone, "-l": FlagArgNone, "-R": FlagArgNone,
		"-S": FlagArgString, "-G": FlagArgString, "-O": FlagArgString,
	})},
	"git log": {SafeFlags: mergeFlags(gitLogDisplayFlags, gitRefFlags, gitDateFlags, gitCountFlags,
		gitStatFlags, gitColorFlags, gitPatchFlags, gitAuthorFlags, map[string]FlagArgType{
			"--abbrev-commit": FlagArgNone, "--full-history": FlagArgNone,
			"--first-parent": FlagArgNone, "--merges": FlagArgNone, "--no-merges": FlagArgNone,
			"--reverse": FlagArgNone, "--follow": FlagArgNone, "--topo-order": FlagArgNone,
			"--date-order": FlagArgNone, "--author-date-order": FlagArgNone,
			"--pretty": FlagArgString, "--format": FlagArgString, "--skip": FlagArgNumber,
			"--diff-filter": FlagArgString, "-S": FlagArgString, "-G": FlagArgString,
		})},
	"git show": {SafeFlags: mergeFlags(gitLogDisplayFlags, gitStatFlags, gitColorFlags, gitPatchFlags,
		map[string]FlagArgType{
			"--abbrev-commit": FlagArgNone, "--word-diff": FlagArgNone,
			"--pretty": FlagArgString, "--format": FlagArgString,
			"--first-parent": FlagArgNone, "--raw": FlagArgNone,
			"-m": FlagArgNone, "--quiet": FlagArgNone,
		})},
	"git status": {SafeFlags: map[string]FlagArgType{
		"--short": FlagArgNone, "-s": FlagArgNone, "--branch": FlagArgNone, "-b": FlagArgNone,
		"--porcelain": FlagArgNone, "--long": FlagArgNone, "--ahead-behind": FlagArgNone,
		"--no-ahead-behind": FlagArgNone, "--renames": FlagArgNone, "--no-renames": FlagArgNone,
		"-u": FlagArgNone, "--untracked-files": FlagArgNone,
	}},
	"git blame": {SafeFlags: map[string]FlagArgType{
		"-L": FlagArgString, "--line-porcelain": FlagArgNone, "--porcelain": FlagArgNone,
		"-p": FlagArgNone, "--show-stats": FlagArgNone, "-n": FlagArgNone,
		"--show-name": FlagArgNone, "-s": FlagArgNone, "-e": FlagArgNone,
		"--show-email": FlagArgNone, "-w": FlagArgNone, "--abbrev": FlagArgNumber,
		"--root": FlagArgNone,
	}},
	"git branch": {SafeFlags: map[string]FlagArgType{
		"-l": FlagArgNone, "--list": FlagArgNone, "-a": FlagArgNone, "--all": FlagArgNone,
		"-r": FlagArgNone, "--remotes": FlagArgNone, "-v": FlagArgNone, "--verbose": FlagArgNone,
		"-vv": FlagArgNone, "--merged": FlagArgNone, "--no-merged": FlagArgNone,
		"--format": FlagArgString, "--sort": FlagArgString, "--color": FlagArgNone,
		"--no-color": FlagArgNone,
	}},
	"git tag": {SafeFlags: map[string]FlagArgType{
		"-l": FlagArgNone, "--list": FlagArgNone, "--sort": FlagArgString,
		"--format": FlagArgString, "--color": FlagArgNone, "--no-color": FlagArgNone,
		"-n": FlagArgNumber, "--column": FlagArgNone, "--no-column": FlagArgNone,
	}},
	"git remote":    {SafeFlags: map[string]FlagArgType{"-v": FlagArgNone, "--verbose": FlagArgNone}},
	"git remote -v": {SafeFlags: map[string]FlagArgType{}},
	"git describe": {SafeFlags: map[string]FlagArgType{
		"--all": FlagArgNone, "--tags": FlagArgNone, "--always": FlagArgNone,
		"--long": FlagArgNone, "--abbrev": FlagArgNumber, "--match": FlagArgString,
		"--exact-match": FlagArgNone, "--dirty": FlagArgNone, "--first-parent": FlagArgNone,
	}},
	"git rev-parse": {SafeFlags: map[string]FlagArgType{
		"--abbrev-ref": FlagArgNone, "--short": FlagArgNone, "--verify": FlagArgNone,
		"--symbolic": FlagArgNone, "--symbolic-full-name": FlagArgNone,
		"--is-inside-work-tree": FlagArgNone, "--show-toplevel": FlagArgNone,
		"--git-dir": FlagArgNone, "--is-bare-repository": FlagArgNone,
	}},
	"git ls-files": {SafeFlags: map[string]FlagArgType{
		"-c": FlagArgNone, "--cached": FlagArgNone, "-d": FlagArgNone, "--deleted": FlagArgNone,
		"-m": FlagArgNone, "--modified": FlagArgNone, "-o": FlagArgNone, "--others": FlagArgNone,
		"-s": FlagArgNone, "--stage": FlagArgNone, "-v": FlagArgNone,
		"--error-unmatch": FlagArgNone, "-z": FlagArgNone,
		"--exclude-standard": FlagArgNone,
	}},
	"git cat-file": {SafeFlags: map[string]FlagArgType{
		"-t": FlagArgNone, "-s": FlagArgNone, "-e": FlagArgNone, "-p": FlagArgNone,
		"--batch": FlagArgNone, "--batch-check": FlagArgNone,
	}},
	"git config": {SafeFlags: map[string]FlagArgType{
		"--get": FlagArgNone, "--list": FlagArgNone, "-l": FlagArgNone,
		"--global": FlagArgNone, "--local": FlagArgNone, "--system": FlagArgNone,
		"--name-only": FlagArgNone, "--null": FlagArgNone, "-z": FlagArgNone,
	}},
	"git reflog": {
		SafeFlags: mergeFlags(gitLogDisplayFlags, gitRefFlags, gitDateFlags, gitCountFlags, gitAuthorFlags),
		IsDangerous: func(_ string, args []string) bool {
			dangerous := map[string]bool{"expire": true, "delete": true, "exists": true}
			for _, t := range args {
				if t == "" || t[0] == '-' {
					continue
				}
				return dangerous[t]
			}
			return false
		},
	},
}
