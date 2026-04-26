// Package shell — git 읽기 전용 명령 설정 맵.
//
// 파일 역할: GITReadOnlyCommands 맵을 정의한다.
//
//	git diff/log/show/status/branch/tag/remote/ls-files/grep/blame/stash/
//	describe/rev-parse/rev-list/cat-file/for-each-ref/reflog 등의
//	읽기 전용 서브커맨드와 각각의 안전 플래그를 포함한다.
//
// 포함 모듈:
//   - GITReadOnlyCommands: git 서브커맨드 → ExternalCommandConfig 맵
//
// 호출/사용 방식:
//   - internal/tools/bash 에서 git 명령 허가 검사 시 조회
//   - internal/tools/powershell 에서도 동일하게 사용
//
// 연결:
//   - 원본: src/utils/shell/readOnlyCommandValidation.ts (GIT_READ_ONLY_COMMANDS)
package shell

import (
	"regexp"
	"strings"
)

// boolPtr 은 bool 값의 포인터를 반환한다 (RespectsDoubleDash 설정용).
func boolPtr(b bool) *bool { return &b }

// GITReadOnlyCommands 는 git 읽기 전용 서브커맨드의 안전 플래그 설정 맵이다.
var GITReadOnlyCommands = map[string]ExternalCommandConfig{
	"git diff": {
		SafeFlags: map[string]FlagArgType{
			"--stat": FlagArgTypeNone, "--numstat": FlagArgTypeNone, "--shortstat": FlagArgTypeNone,
			"--name-only": FlagArgTypeNone, "--name-status": FlagArgTypeNone,
			"--color": FlagArgTypeNone, "--no-color": FlagArgTypeNone,
			"--dirstat": FlagArgTypeNone, "--summary": FlagArgTypeNone, "--patch-with-stat": FlagArgTypeNone,
			"--word-diff": FlagArgTypeNone, "--word-diff-regex": FlagArgTypeString,
			"--color-words": FlagArgTypeNone, "--no-renames": FlagArgTypeNone,
			"--no-ext-diff": FlagArgTypeNone, "--check": FlagArgTypeNone,
			"--ws-error-highlight": FlagArgTypeString, "--full-index": FlagArgTypeNone,
			"--binary": FlagArgTypeNone, "--abbrev": FlagArgTypeNumber,
			"--break-rewrites": FlagArgTypeNone, "--find-renames": FlagArgTypeNone,
			"--find-copies": FlagArgTypeNone, "--find-copies-harder": FlagArgTypeNone,
			"--irreversible-delete": FlagArgTypeNone, "--diff-algorithm": FlagArgTypeString,
			"--histogram": FlagArgTypeNone, "--patience": FlagArgTypeNone, "--minimal": FlagArgTypeNone,
			"--ignore-space-at-eol": FlagArgTypeNone, "--ignore-space-change": FlagArgTypeNone,
			"--ignore-all-space": FlagArgTypeNone, "--ignore-blank-lines": FlagArgTypeNone,
			"--inter-hunk-context": FlagArgTypeNumber, "--function-context": FlagArgTypeNone,
			"--exit-code": FlagArgTypeNone, "--quiet": FlagArgTypeNone,
			"--cached": FlagArgTypeNone, "--staged": FlagArgTypeNone,
			"--pickaxe-regex": FlagArgTypeNone, "--pickaxe-all": FlagArgTypeNone,
			"--no-index": FlagArgTypeNone, "--relative": FlagArgTypeString,
			"--diff-filter": FlagArgTypeString,
			"-p":            FlagArgTypeNone, "-u": FlagArgTypeNone, "-s": FlagArgTypeNone,
			"-M": FlagArgTypeNone, "-C": FlagArgTypeNone, "-B": FlagArgTypeNone,
			"-D": FlagArgTypeNone, "-l": FlagArgTypeNone,
			"-S": FlagArgTypeString, "-G": FlagArgTypeString, "-O": FlagArgTypeString,
			"-R": FlagArgTypeNone,
		},
	},
	"git log": {
		SafeFlags: map[string]FlagArgType{
			"--oneline": FlagArgTypeNone, "--graph": FlagArgTypeNone, "--decorate": FlagArgTypeNone,
			"--no-decorate": FlagArgTypeNone, "--date": FlagArgTypeString, "--relative-date": FlagArgTypeNone,
			"--all": FlagArgTypeNone, "--branches": FlagArgTypeNone, "--tags": FlagArgTypeNone, "--remotes": FlagArgTypeNone,
			"--since": FlagArgTypeString, "--after": FlagArgTypeString, "--until": FlagArgTypeString, "--before": FlagArgTypeString,
			"--max-count": FlagArgTypeNumber, "-n": FlagArgTypeNumber,
			"--stat": FlagArgTypeNone, "--numstat": FlagArgTypeNone, "--shortstat": FlagArgTypeNone,
			"--name-only": FlagArgTypeNone, "--name-status": FlagArgTypeNone,
			"--color": FlagArgTypeNone, "--no-color": FlagArgTypeNone,
			"--patch": FlagArgTypeNone, "-p": FlagArgTypeNone, "--no-patch": FlagArgTypeNone,
			"--no-ext-diff": FlagArgTypeNone, "-s": FlagArgTypeNone,
			"--author": FlagArgTypeString, "--committer": FlagArgTypeString, "--grep": FlagArgTypeString,
			"--abbrev-commit": FlagArgTypeNone, "--full-history": FlagArgTypeNone,
			"--dense": FlagArgTypeNone, "--sparse": FlagArgTypeNone,
			"--simplify-merges": FlagArgTypeNone, "--ancestry-path": FlagArgTypeNone,
			"--source": FlagArgTypeNone, "--first-parent": FlagArgTypeNone,
			"--merges": FlagArgTypeNone, "--no-merges": FlagArgTypeNone,
			"--reverse": FlagArgTypeNone, "--walk-reflogs": FlagArgTypeNone,
			"--skip": FlagArgTypeNumber, "--max-age": FlagArgTypeNumber, "--min-age": FlagArgTypeNumber,
			"--no-min-parents": FlagArgTypeNone, "--no-max-parents": FlagArgTypeNone,
			"--follow": FlagArgTypeNone, "--no-walk": FlagArgTypeNone,
			"--left-right": FlagArgTypeNone, "--cherry-mark": FlagArgTypeNone,
			"--cherry-pick": FlagArgTypeNone, "--boundary": FlagArgTypeNone,
			"--topo-order": FlagArgTypeNone, "--date-order": FlagArgTypeNone, "--author-date-order": FlagArgTypeNone,
			"--pretty": FlagArgTypeString, "--format": FlagArgTypeString,
			"--diff-filter": FlagArgTypeString,
			"-S":            FlagArgTypeString, "-G": FlagArgTypeString,
			"--pickaxe-regex": FlagArgTypeNone, "--pickaxe-all": FlagArgTypeNone,
		},
	},
	"git show": {
		SafeFlags: map[string]FlagArgType{
			"--oneline": FlagArgTypeNone, "--graph": FlagArgTypeNone, "--decorate": FlagArgTypeNone,
			"--no-decorate": FlagArgTypeNone, "--date": FlagArgTypeString, "--relative-date": FlagArgTypeNone,
			"--stat": FlagArgTypeNone, "--numstat": FlagArgTypeNone, "--shortstat": FlagArgTypeNone,
			"--name-only": FlagArgTypeNone, "--name-status": FlagArgTypeNone,
			"--color": FlagArgTypeNone, "--no-color": FlagArgTypeNone,
			"--patch": FlagArgTypeNone, "-p": FlagArgTypeNone, "--no-patch": FlagArgTypeNone,
			"--no-ext-diff": FlagArgTypeNone, "-s": FlagArgTypeNone,
			"--abbrev-commit": FlagArgTypeNone, "--word-diff": FlagArgTypeNone,
			"--word-diff-regex": FlagArgTypeString, "--color-words": FlagArgTypeNone,
			"--pretty": FlagArgTypeString, "--format": FlagArgTypeString,
			"--first-parent": FlagArgTypeNone, "--raw": FlagArgTypeNone,
			"--diff-filter": FlagArgTypeString,
			"-m":            FlagArgTypeNone, "--quiet": FlagArgTypeNone,
		},
	},
	"git status": {
		SafeFlags: map[string]FlagArgType{
			"--short": FlagArgTypeNone, "-s": FlagArgTypeNone,
			"--branch": FlagArgTypeNone, "-b": FlagArgTypeNone,
			"--porcelain": FlagArgTypeNone, "--long": FlagArgTypeNone,
			"--verbose": FlagArgTypeNone, "-v": FlagArgTypeNone,
			"--untracked-files": FlagArgTypeString, "-u": FlagArgTypeString,
			"--ignored": FlagArgTypeNone, "--ignore-submodules": FlagArgTypeString,
			"--column": FlagArgTypeNone, "--no-column": FlagArgTypeNone,
			"--ahead-behind": FlagArgTypeNone, "--no-ahead-behind": FlagArgTypeNone,
			"--renames": FlagArgTypeNone, "--no-renames": FlagArgTypeNone,
			"--find-renames": FlagArgTypeString, "-M": FlagArgTypeString,
		},
	},
	"git branch": {
		SafeFlags: map[string]FlagArgType{
			"-l": FlagArgTypeNone, "--list": FlagArgTypeNone,
			"-a": FlagArgTypeNone, "--all": FlagArgTypeNone,
			"-r": FlagArgTypeNone, "--remotes": FlagArgTypeNone,
			"-v": FlagArgTypeNone, "-vv": FlagArgTypeNone, "--verbose": FlagArgTypeNone,
			"--color": FlagArgTypeNone, "--no-color": FlagArgTypeNone,
			"--column": FlagArgTypeNone, "--no-column": FlagArgTypeNone,
			"--abbrev": FlagArgTypeNumber, "--no-abbrev": FlagArgTypeNone,
			"--contains": FlagArgTypeString, "--no-contains": FlagArgTypeString,
			"--merged": FlagArgTypeNone, "--no-merged": FlagArgTypeNone,
			"--points-at": FlagArgTypeString, "--sort": FlagArgTypeString,
			"--show-current": FlagArgTypeNone, "-i": FlagArgTypeNone, "--ignore-case": FlagArgTypeNone,
		},
		AdditionalCommandIsDangerous: func(_ string, args []string) bool {
			flagsWithArgs := map[string]bool{"--contains": true, "--no-contains": true, "--points-at": true, "--sort": true}
			flagsWithOptionalArgs := map[string]bool{"--merged": true, "--no-merged": true}
			i := 0
			lastFlag := ""
			seenListFlag := false
			seenDashDash := false
			for i < len(args) {
				token := args[i]
				if token == "" {
					i++
					continue
				}
				if token == "--" && !seenDashDash {
					seenDashDash = true
					lastFlag = ""
					i++
					continue
				}
				if !seenDashDash && strings.HasPrefix(token, "-") {
					if token == "--list" || token == "-l" {
						seenListFlag = true
					} else if len(token) > 2 && token[1] != '-' && !strings.Contains(token, "=") && strings.ContainsRune(token[1:], 'l') {
						seenListFlag = true
					}
					if before, _, hasEq := strings.Cut(token, "="); hasEq {
						lastFlag = before
						i++
					} else if flagsWithArgs[token] {
						lastFlag = token
						i += 2
					} else {
						lastFlag = token
						i++
					}
				} else {
					if !seenListFlag && !flagsWithOptionalArgs[lastFlag] {
						return true
					}
					i++
				}
			}
			return false
		},
	},
	"git tag": {
		SafeFlags: map[string]FlagArgType{
			"-l": FlagArgTypeNone, "--list": FlagArgTypeNone,
			"-n":         FlagArgTypeNumber,
			"--contains": FlagArgTypeString, "--no-contains": FlagArgTypeString,
			"--merged": FlagArgTypeString, "--no-merged": FlagArgTypeString,
			"--sort": FlagArgTypeString, "--format": FlagArgTypeString,
			"--points-at": FlagArgTypeString,
			"--column":    FlagArgTypeNone, "--no-column": FlagArgTypeNone,
			"-i": FlagArgTypeNone, "--ignore-case": FlagArgTypeNone,
		},
		AdditionalCommandIsDangerous: func(_ string, args []string) bool {
			flagsWithArgs := map[string]bool{"--contains": true, "--no-contains": true, "--merged": true, "--no-merged": true, "--points-at": true, "--sort": true, "--format": true, "-n": true}
			i := 0
			seenListFlag := false
			seenDashDash := false
			for i < len(args) {
				token := args[i]
				if token == "" {
					i++
					continue
				}
				if token == "--" && !seenDashDash {
					seenDashDash = true
					i++
					continue
				}
				if !seenDashDash && strings.HasPrefix(token, "-") {
					if token == "--list" || token == "-l" {
						seenListFlag = true
					} else if len(token) > 2 && token[1] != '-' && !strings.Contains(token, "=") && strings.ContainsRune(token[1:], 'l') {
						seenListFlag = true
					}
					if strings.Contains(token, "=") {
						i++
					} else if flagsWithArgs[token] {
						i += 2
					} else {
						i++
					}
				} else {
					if !seenListFlag {
						return true
					}
					i++
				}
			}
			return false
		},
	},
	"git remote show": {
		SafeFlags: map[string]FlagArgType{"-n": FlagArgTypeNone},
		AdditionalCommandIsDangerous: func(_ string, args []string) bool {
			var positional []string
			for _, a := range args {
				if a != "-n" {
					positional = append(positional, a)
				}
			}
			if len(positional) != 1 {
				return true
			}
			matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]+$`, positional[0])
			return !matched
		},
	},
	"git remote": {
		SafeFlags: map[string]FlagArgType{"-v": FlagArgTypeNone, "--verbose": FlagArgTypeNone},
		AdditionalCommandIsDangerous: func(_ string, args []string) bool {
			for _, a := range args {
				if a != "-v" && a != "--verbose" {
					return true
				}
			}
			return false
		},
	},
	"git ls-files": {
		SafeFlags: map[string]FlagArgType{
			"--cached": FlagArgTypeNone, "-c": FlagArgTypeNone,
			"--deleted": FlagArgTypeNone, "-d": FlagArgTypeNone,
			"--modified": FlagArgTypeNone, "-m": FlagArgTypeNone,
			"--others": FlagArgTypeNone, "-o": FlagArgTypeNone,
			"--ignored": FlagArgTypeNone, "-i": FlagArgTypeNone,
			"--stage": FlagArgTypeNone, "-s": FlagArgTypeNone,
			"--killed": FlagArgTypeNone, "-k": FlagArgTypeNone,
			"--unmerged": FlagArgTypeNone, "-u": FlagArgTypeNone,
			"--directory": FlagArgTypeNone, "--no-empty-directory": FlagArgTypeNone,
			"--eol": FlagArgTypeNone, "--full-name": FlagArgTypeNone,
			"--abbrev": FlagArgTypeNumber, "--debug": FlagArgTypeNone,
			"-z": FlagArgTypeNone, "-t": FlagArgTypeNone, "-v": FlagArgTypeNone, "-f": FlagArgTypeNone,
			"--exclude": FlagArgTypeString, "-x": FlagArgTypeString,
			"--exclude-from": FlagArgTypeString, "-X": FlagArgTypeString,
			"--exclude-per-directory": FlagArgTypeString, "--exclude-standard": FlagArgTypeNone,
			"--error-unmatch": FlagArgTypeNone, "--recurse-submodules": FlagArgTypeNone,
		},
	},
	"git grep": {
		SafeFlags: map[string]FlagArgType{
			"-e": FlagArgTypeString, "-E": FlagArgTypeNone, "--extended-regexp": FlagArgTypeNone,
			"-G": FlagArgTypeNone, "--basic-regexp": FlagArgTypeNone,
			"-F": FlagArgTypeNone, "--fixed-strings": FlagArgTypeNone,
			"-P": FlagArgTypeNone, "--perl-regexp": FlagArgTypeNone,
			"-i": FlagArgTypeNone, "--ignore-case": FlagArgTypeNone,
			"-v": FlagArgTypeNone, "--invert-match": FlagArgTypeNone,
			"-w": FlagArgTypeNone, "--word-regexp": FlagArgTypeNone,
			"-n": FlagArgTypeNone, "--line-number": FlagArgTypeNone,
			"-c": FlagArgTypeNone, "--count": FlagArgTypeNone,
			"-l": FlagArgTypeNone, "--files-with-matches": FlagArgTypeNone,
			"-L": FlagArgTypeNone, "--files-without-match": FlagArgTypeNone,
			"-h": FlagArgTypeNone, "-H": FlagArgTypeNone,
			"--heading": FlagArgTypeNone, "--break": FlagArgTypeNone,
			"--full-name": FlagArgTypeNone,
			"--color":     FlagArgTypeNone, "--no-color": FlagArgTypeNone,
			"-o": FlagArgTypeNone, "--only-matching": FlagArgTypeNone,
			"-A": FlagArgTypeNumber, "--after-context": FlagArgTypeNumber,
			"-B": FlagArgTypeNumber, "--before-context": FlagArgTypeNumber,
			"-C": FlagArgTypeNumber, "--context": FlagArgTypeNumber,
			"--and": FlagArgTypeNone, "--or": FlagArgTypeNone, "--not": FlagArgTypeNone,
			"--max-depth": FlagArgTypeNumber, "--untracked": FlagArgTypeNone,
			"--no-index": FlagArgTypeNone, "--recurse-submodules": FlagArgTypeNone,
			"--cached": FlagArgTypeNone, "--threads": FlagArgTypeNumber,
			"-q": FlagArgTypeNone, "--quiet": FlagArgTypeNone,
		},
	},
	"git blame": {
		SafeFlags: map[string]FlagArgType{
			"--color": FlagArgTypeNone, "--no-color": FlagArgTypeNone,
			"-L":          FlagArgTypeString,
			"--porcelain": FlagArgTypeNone, "-p": FlagArgTypeNone,
			"--line-porcelain": FlagArgTypeNone, "--incremental": FlagArgTypeNone,
			"--root": FlagArgTypeNone, "--show-stats": FlagArgTypeNone,
			"--show-name": FlagArgTypeNone, "--show-number": FlagArgTypeNone,
			"-n": FlagArgTypeNone, "--show-email": FlagArgTypeNone, "-e": FlagArgTypeNone, "-f": FlagArgTypeNone,
			"--date": FlagArgTypeString, "-w": FlagArgTypeNone,
			"--ignore-rev": FlagArgTypeString, "--ignore-revs-file": FlagArgTypeString,
			"-M": FlagArgTypeNone, "-C": FlagArgTypeNone, "--score-debug": FlagArgTypeNone,
			"--abbrev": FlagArgTypeNumber, "-s": FlagArgTypeNone, "-l": FlagArgTypeNone, "-t": FlagArgTypeNone,
		},
	},
	"git stash list": {
		SafeFlags: map[string]FlagArgType{
			"--oneline": FlagArgTypeNone, "--graph": FlagArgTypeNone, "--decorate": FlagArgTypeNone,
			"--no-decorate": FlagArgTypeNone, "--date": FlagArgTypeString, "--relative-date": FlagArgTypeNone,
			"--all": FlagArgTypeNone, "--branches": FlagArgTypeNone, "--tags": FlagArgTypeNone, "--remotes": FlagArgTypeNone,
			"--max-count": FlagArgTypeNumber, "-n": FlagArgTypeNumber,
		},
	},
	"git stash show": {
		SafeFlags: map[string]FlagArgType{
			"--stat": FlagArgTypeNone, "--numstat": FlagArgTypeNone, "--shortstat": FlagArgTypeNone,
			"--name-only": FlagArgTypeNone, "--name-status": FlagArgTypeNone,
			"--color": FlagArgTypeNone, "--no-color": FlagArgTypeNone,
			"--patch": FlagArgTypeNone, "-p": FlagArgTypeNone, "--no-patch": FlagArgTypeNone,
			"--no-ext-diff": FlagArgTypeNone, "-s": FlagArgTypeNone,
			"--word-diff": FlagArgTypeNone, "--word-diff-regex": FlagArgTypeString,
			"--diff-filter": FlagArgTypeString, "--abbrev": FlagArgTypeNumber,
		},
	},
	"git describe": {
		SafeFlags: map[string]FlagArgType{
			"--tags": FlagArgTypeNone, "--match": FlagArgTypeString, "--exclude": FlagArgTypeString,
			"--long": FlagArgTypeNone, "--abbrev": FlagArgTypeNumber, "--always": FlagArgTypeNone,
			"--contains": FlagArgTypeNone, "--first-match": FlagArgTypeNone, "--exact-match": FlagArgTypeNone,
			"--candidates": FlagArgTypeNumber, "--dirty": FlagArgTypeNone, "--broken": FlagArgTypeNone,
			"--all": FlagArgTypeNone,
		},
	},
	"git rev-parse": {
		SafeFlags: map[string]FlagArgType{
			"--verify": FlagArgTypeNone, "--short": FlagArgTypeString, "--abbrev-ref": FlagArgTypeNone,
			"--symbolic": FlagArgTypeNone, "--symbolic-full-name": FlagArgTypeNone,
			"--show-toplevel": FlagArgTypeNone, "--show-cdup": FlagArgTypeNone, "--show-prefix": FlagArgTypeNone,
			"--git-dir": FlagArgTypeNone, "--git-common-dir": FlagArgTypeNone,
			"--absolute-git-dir": FlagArgTypeNone, "--show-superproject-working-tree": FlagArgTypeNone,
			"--is-inside-work-tree": FlagArgTypeNone, "--is-inside-git-dir": FlagArgTypeNone,
			"--is-bare-repository": FlagArgTypeNone, "--is-shallow-repository": FlagArgTypeNone,
			"--is-shallow-update": FlagArgTypeNone, "--path-prefix": FlagArgTypeNone,
			"--quiet": FlagArgTypeNone,
		},
	},
	"git shortlog": {
		SafeFlags: map[string]FlagArgType{
			"--all": FlagArgTypeNone, "--branches": FlagArgTypeNone, "--tags": FlagArgTypeNone, "--remotes": FlagArgTypeNone,
			"--since": FlagArgTypeString, "--after": FlagArgTypeString, "--until": FlagArgTypeString, "--before": FlagArgTypeString,
			"-s": FlagArgTypeNone, "--summary": FlagArgTypeNone,
			"-n": FlagArgTypeNone, "--numbered": FlagArgTypeNone,
			"-e": FlagArgTypeNone, "--email": FlagArgTypeNone,
			"-c": FlagArgTypeNone, "--committer": FlagArgTypeString,
			"--group": FlagArgTypeString, "--format": FlagArgTypeString,
			"--no-merges": FlagArgTypeNone, "--author": FlagArgTypeString,
		},
	},
	"git reflog": {
		SafeFlags: map[string]FlagArgType{
			"--oneline": FlagArgTypeNone, "--graph": FlagArgTypeNone, "--decorate": FlagArgTypeNone,
			"--no-decorate": FlagArgTypeNone, "--date": FlagArgTypeString, "--relative-date": FlagArgTypeNone,
			"--all": FlagArgTypeNone, "--branches": FlagArgTypeNone, "--tags": FlagArgTypeNone, "--remotes": FlagArgTypeNone,
			"--since": FlagArgTypeString, "--after": FlagArgTypeString, "--until": FlagArgTypeString, "--before": FlagArgTypeString,
			"--max-count": FlagArgTypeNumber, "-n": FlagArgTypeNumber,
			"--author": FlagArgTypeString, "--committer": FlagArgTypeString, "--grep": FlagArgTypeString,
		},
		AdditionalCommandIsDangerous: func(_ string, args []string) bool {
			dangerous := map[string]bool{"expire": true, "delete": true, "exists": true}
			for _, token := range args {
				if token == "" || strings.HasPrefix(token, "-") {
					continue
				}
				return dangerous[token]
			}
			return false
		},
	},
	"git ls-tree": {
		SafeFlags: map[string]FlagArgType{
			"-r": FlagArgTypeNone, "-l": FlagArgTypeNone, "-d": FlagArgTypeNone, "-t": FlagArgTypeNone,
			"--name-only": FlagArgTypeNone, "--full-name": FlagArgTypeNone, "--full-tree": FlagArgTypeNone,
		},
	},
	"git cat-file": {
		SafeFlags: map[string]FlagArgType{
			"-t": FlagArgTypeNone, "-s": FlagArgTypeNone, "-p": FlagArgTypeNone, "-e": FlagArgTypeNone,
			"--batch-check": FlagArgTypeNone, "--allow-undetermined-type": FlagArgTypeNone,
		},
	},
	"git for-each-ref": {
		SafeFlags: map[string]FlagArgType{
			"--format": FlagArgTypeString, "--sort": FlagArgTypeString, "--count": FlagArgTypeNumber,
			"--contains": FlagArgTypeString, "--no-contains": FlagArgTypeString,
			"--merged": FlagArgTypeString, "--no-merged": FlagArgTypeString,
			"--points-at": FlagArgTypeString,
		},
	},
	"git rev-list": {
		SafeFlags: map[string]FlagArgType{
			"--all": FlagArgTypeNone, "--branches": FlagArgTypeNone, "--tags": FlagArgTypeNone, "--remotes": FlagArgTypeNone,
			"--since": FlagArgTypeString, "--after": FlagArgTypeString, "--until": FlagArgTypeString, "--before": FlagArgTypeString,
			"--max-count": FlagArgTypeNumber, "-n": FlagArgTypeNumber,
			"--author": FlagArgTypeString, "--committer": FlagArgTypeString, "--grep": FlagArgTypeString,
			"--count": FlagArgTypeNone, "--reverse": FlagArgTypeNone, "--first-parent": FlagArgTypeNone,
			"--ancestry-path": FlagArgTypeNone, "--merges": FlagArgTypeNone, "--no-merges": FlagArgTypeNone,
			"--min-parents": FlagArgTypeNumber, "--max-parents": FlagArgTypeNumber,
			"--no-min-parents": FlagArgTypeNone, "--no-max-parents": FlagArgTypeNone,
			"--skip": FlagArgTypeNumber, "--max-age": FlagArgTypeNumber, "--min-age": FlagArgTypeNumber,
			"--walk-reflogs": FlagArgTypeNone, "--oneline": FlagArgTypeNone, "--abbrev-commit": FlagArgTypeNone,
			"--pretty": FlagArgTypeString, "--format": FlagArgTypeString, "--abbrev": FlagArgTypeNumber,
			"--full-history": FlagArgTypeNone, "--dense": FlagArgTypeNone, "--sparse": FlagArgTypeNone,
			"--source": FlagArgTypeNone, "--graph": FlagArgTypeNone,
		},
	},
	"git worktree list": {
		SafeFlags: map[string]FlagArgType{
			"--porcelain": FlagArgTypeNone, "-v": FlagArgTypeNone, "--verbose": FlagArgTypeNone,
			"--expire": FlagArgTypeString,
		},
	},
	"git ls-remote": {
		SafeFlags: map[string]FlagArgType{
			"--branches": FlagArgTypeNone, "-b": FlagArgTypeNone,
			"--tags": FlagArgTypeNone, "-t": FlagArgTypeNone,
			"--heads": FlagArgTypeNone, "-h": FlagArgTypeNone,
			"--refs": FlagArgTypeNone, "--quiet": FlagArgTypeNone, "-q": FlagArgTypeNone,
			"--exit-code": FlagArgTypeNone, "--get-url": FlagArgTypeNone, "--symref": FlagArgTypeNone,
			"--sort": FlagArgTypeString,
		},
	},
	"git config --get": {
		SafeFlags: map[string]FlagArgType{
			"--local": FlagArgTypeNone, "--global": FlagArgTypeNone,
			"--system": FlagArgTypeNone, "--worktree": FlagArgTypeNone,
			"--default": FlagArgTypeString, "--type": FlagArgTypeString,
			"--bool": FlagArgTypeNone, "--int": FlagArgTypeNone,
			"--bool-or-int": FlagArgTypeNone, "--path": FlagArgTypeNone,
			"--expiry-date": FlagArgTypeNone, "-z": FlagArgTypeNone,
			"--null": FlagArgTypeNone, "--name-only": FlagArgTypeNone,
			"--show-origin": FlagArgTypeNone, "--show-scope": FlagArgTypeNone,
		},
	},
	"git merge-base": {
		SafeFlags: map[string]FlagArgType{
			"--is-ancestor": FlagArgTypeNone, "--fork-point": FlagArgTypeNone,
			"--octopus": FlagArgTypeNone, "--independent": FlagArgTypeNone, "--all": FlagArgTypeNone,
		},
	},
}
