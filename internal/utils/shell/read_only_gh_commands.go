// Package shell — gh CLI / 외부 읽기 전용 명령 설정 맵.
//
// 파일 역할: GHReadOnlyCommands (gh CLI 서브커맨드) 와 ExternalReadonlyCommands
//
//	(grep, rg, find, cat, jq, curl 등 범용 읽기 전용 도구) 설정 맵을 정의한다.
//	또한 docker, ripgrep, pyright 전용 맵과 ExternalReadonlyCommandNames 슬라이스도 포함한다.
//
// 포함 모듈:
//   - GHReadOnlyCommands: gh pr/issue/repo/run/workflow/release/auth/search 읽기 전용 서브커맨드 맵
//   - DockerReadOnlyCommands: docker logs/inspect 읽기 전용 맵
//   - RipgrepReadOnlyCommands: rg (ripgrep) 읽기 전용 맵
//   - PyrightReadOnlyCommands: pyright 정적 타입 체커 읽기 전용 맵
//   - ExternalReadonlyCommandNames: 크로스 셸 읽기 전용 명령 이름 슬라이스
//
// 호출/사용 방식:
//   - internal/tools/bash 에서 외부 명령 허가 검사 시 조회
//   - internal/tools/powershell 에서도 동일하게 사용
//
// 연결:
//   - 원본: src/utils/shell/readOnlyCommandValidation.ts (GH_READ_ONLY_COMMANDS, EXTERNAL_READONLY_COMMANDS)
package shell

import (
	"strings"
)

// ghIsDangerous 는 gh 명령에서 HOST/OWNER/REPO 형식이나 URL을 포함하는지 검사하는 공통 콜백이다.
// gh 의 --repo 플래그 등에서 DNS 기반 비밀 유출(exfiltration)을 방지한다.
func ghIsDangerous(_ string, args []string) bool {
	for _, token := range args {
		if token == "" {
			continue
		}
		value := token
		if strings.HasPrefix(token, "-") {
			var after string
			var found bool
			_, after, found = strings.Cut(token, "=")
			if !found {
				continue
			}
			value = after
			if value == "" {
				continue
			}
		}
		if !strings.Contains(value, "/") && !strings.Contains(value, "://") && !strings.Contains(value, "@") {
			continue
		}
		if strings.Contains(value, "://") {
			return true
		}
		if strings.Contains(value, "@") {
			return true
		}
		// OWNER/REPO 는 슬래시가 1개, HOST/OWNER/REPO 는 2개 이상
		slashCount := strings.Count(value, "/")
		if slashCount >= 2 {
			return true
		}
	}
	return false
}

// GHReadOnlyCommands 는 gh CLI 읽기 전용 서브커맨드의 안전 플래그 설정 맵이다.
var GHReadOnlyCommands = map[string]ExternalCommandConfig{
	"gh pr view": {
		SafeFlags: map[string]FlagArgType{
			"--json": FlagArgTypeString, "--comments": FlagArgTypeNone,
			"--repo": FlagArgTypeString, "-R": FlagArgTypeString,
		},
		AdditionalCommandIsDangerous: ghIsDangerous,
	},
	"gh pr list": {
		SafeFlags: map[string]FlagArgType{
			"--state": FlagArgTypeString, "-s": FlagArgTypeString,
			"--author": FlagArgTypeString, "--assignee": FlagArgTypeString,
			"--label": FlagArgTypeString, "--limit": FlagArgTypeNumber, "-L": FlagArgTypeNumber,
			"--base": FlagArgTypeString, "--head": FlagArgTypeString, "--search": FlagArgTypeString,
			"--json": FlagArgTypeString, "--draft": FlagArgTypeNone, "--app": FlagArgTypeString,
			"--repo": FlagArgTypeString, "-R": FlagArgTypeString,
		},
		AdditionalCommandIsDangerous: ghIsDangerous,
	},
	"gh pr diff": {
		SafeFlags: map[string]FlagArgType{
			"--color": FlagArgTypeString, "--name-only": FlagArgTypeNone,
			"--patch": FlagArgTypeNone, "--repo": FlagArgTypeString, "-R": FlagArgTypeString,
		},
		AdditionalCommandIsDangerous: ghIsDangerous,
	},
	"gh pr checks": {
		SafeFlags: map[string]FlagArgType{
			"--watch": FlagArgTypeNone, "--required": FlagArgTypeNone, "--fail-fast": FlagArgTypeNone,
			"--json": FlagArgTypeString, "--interval": FlagArgTypeNumber,
			"--repo": FlagArgTypeString, "-R": FlagArgTypeString,
		},
		AdditionalCommandIsDangerous: ghIsDangerous,
	},
	"gh pr status": {
		SafeFlags: map[string]FlagArgType{
			"--conflict-status": FlagArgTypeNone, "-c": FlagArgTypeNone,
			"--json": FlagArgTypeString, "--repo": FlagArgTypeString, "-R": FlagArgTypeString,
		},
		AdditionalCommandIsDangerous: ghIsDangerous,
	},
	"gh issue view": {
		SafeFlags: map[string]FlagArgType{
			"--json": FlagArgTypeString, "--comments": FlagArgTypeNone,
			"--repo": FlagArgTypeString, "-R": FlagArgTypeString,
		},
		AdditionalCommandIsDangerous: ghIsDangerous,
	},
	"gh issue list": {
		SafeFlags: map[string]FlagArgType{
			"--state": FlagArgTypeString, "-s": FlagArgTypeString,
			"--assignee": FlagArgTypeString, "--author": FlagArgTypeString,
			"--label": FlagArgTypeString, "--limit": FlagArgTypeNumber, "-L": FlagArgTypeNumber,
			"--milestone": FlagArgTypeString, "--search": FlagArgTypeString,
			"--json": FlagArgTypeString, "--app": FlagArgTypeString,
			"--repo": FlagArgTypeString, "-R": FlagArgTypeString,
		},
		AdditionalCommandIsDangerous: ghIsDangerous,
	},
	"gh issue status": {
		SafeFlags: map[string]FlagArgType{
			"--json": FlagArgTypeString, "--repo": FlagArgTypeString, "-R": FlagArgTypeString,
		},
		AdditionalCommandIsDangerous: ghIsDangerous,
	},
	"gh repo view": {
		SafeFlags: map[string]FlagArgType{
			"--json": FlagArgTypeString,
		},
		AdditionalCommandIsDangerous: ghIsDangerous,
	},
	"gh repo list": {
		SafeFlags: map[string]FlagArgType{
			"--json": FlagArgTypeString, "--limit": FlagArgTypeNumber, "-L": FlagArgTypeNumber,
			"--language": FlagArgTypeString, "--source": FlagArgTypeNone, "--fork": FlagArgTypeNone,
			"--archived": FlagArgTypeNone, "--no-archived": FlagArgTypeNone,
			"--visibility": FlagArgTypeString, "--topic": FlagArgTypeString,
		},
		AdditionalCommandIsDangerous: ghIsDangerous,
	},
	"gh run list": {
		SafeFlags: map[string]FlagArgType{
			"--branch": FlagArgTypeString, "-b": FlagArgTypeString,
			"--status": FlagArgTypeString, "-s": FlagArgTypeString,
			"--workflow": FlagArgTypeString, "-w": FlagArgTypeString,
			"--limit": FlagArgTypeNumber, "-L": FlagArgTypeNumber,
			"--json": FlagArgTypeString, "--repo": FlagArgTypeString, "-R": FlagArgTypeString,
			"--event": FlagArgTypeString, "-e": FlagArgTypeString,
			"--user": FlagArgTypeString, "-u": FlagArgTypeString,
			"--created": FlagArgTypeString, "--commit": FlagArgTypeString, "-c": FlagArgTypeString,
		},
		AdditionalCommandIsDangerous: ghIsDangerous,
	},
	"gh run view": {
		SafeFlags: map[string]FlagArgType{
			"--log": FlagArgTypeNone, "--log-failed": FlagArgTypeNone, "--exit-status": FlagArgTypeNone,
			"--verbose": FlagArgTypeNone, "-v": FlagArgTypeNone,
			"--json": FlagArgTypeString, "--repo": FlagArgTypeString, "-R": FlagArgTypeString,
			"--job": FlagArgTypeString, "-j": FlagArgTypeString,
			"--attempt": FlagArgTypeNumber, "-a": FlagArgTypeNumber,
		},
		AdditionalCommandIsDangerous: ghIsDangerous,
	},
	"gh run watch": {
		SafeFlags: map[string]FlagArgType{
			"--interval": FlagArgTypeNumber, "--exit-status": FlagArgTypeNone,
			"--repo": FlagArgTypeString, "-R": FlagArgTypeString,
		},
		AdditionalCommandIsDangerous: ghIsDangerous,
	},
	"gh workflow list": {
		SafeFlags: map[string]FlagArgType{
			"--all": FlagArgTypeNone, "-a": FlagArgTypeNone,
			"--json": FlagArgTypeString, "--limit": FlagArgTypeNumber, "-L": FlagArgTypeNumber,
			"--repo": FlagArgTypeString, "-R": FlagArgTypeString,
		},
		AdditionalCommandIsDangerous: ghIsDangerous,
	},
	"gh workflow view": {
		SafeFlags: map[string]FlagArgType{
			"--ref": FlagArgTypeString, "-r": FlagArgTypeString,
			"--yaml": FlagArgTypeNone, "-y": FlagArgTypeNone,
			"--repo": FlagArgTypeString, "-R": FlagArgTypeString,
		},
		AdditionalCommandIsDangerous: ghIsDangerous,
	},
	"gh release list": {
		SafeFlags: map[string]FlagArgType{
			"--exclude-drafts": FlagArgTypeNone, "--exclude-pre-releases": FlagArgTypeNone,
			"--json": FlagArgTypeString, "--limit": FlagArgTypeNumber, "-L": FlagArgTypeNumber,
			"--order": FlagArgTypeString, "-O": FlagArgTypeString,
			"--repo": FlagArgTypeString, "-R": FlagArgTypeString,
		},
		AdditionalCommandIsDangerous: ghIsDangerous,
	},
	"gh release view": {
		SafeFlags: map[string]FlagArgType{
			"--json": FlagArgTypeString, "--repo": FlagArgTypeString, "-R": FlagArgTypeString,
		},
		AdditionalCommandIsDangerous: ghIsDangerous,
	},
	"gh auth status": {
		SafeFlags: map[string]FlagArgType{
			"--active": FlagArgTypeNone, "-a": FlagArgTypeNone,
			"--hostname": FlagArgTypeString, "-h": FlagArgTypeString,
			"--json": FlagArgTypeString,
		},
		AdditionalCommandIsDangerous: ghIsDangerous,
	},
	"gh label list": {
		SafeFlags: map[string]FlagArgType{
			"--json": FlagArgTypeString, "--limit": FlagArgTypeNumber, "-L": FlagArgTypeNumber,
			"--order": FlagArgTypeString, "--search": FlagArgTypeString, "-S": FlagArgTypeString,
			"--sort": FlagArgTypeString, "--repo": FlagArgTypeString, "-R": FlagArgTypeString,
		},
		AdditionalCommandIsDangerous: ghIsDangerous,
	},
	"gh search repos": {
		SafeFlags: map[string]FlagArgType{
			"--archived": FlagArgTypeNone, "--created": FlagArgTypeString, "--followers": FlagArgTypeString,
			"--forks": FlagArgTypeString, "--good-first-issues": FlagArgTypeString,
			"--help-wanted-issues": FlagArgTypeString, "--include-forks": FlagArgTypeString,
			"--json": FlagArgTypeString, "--language": FlagArgTypeString, "--license": FlagArgTypeString,
			"--limit": FlagArgTypeNumber, "-L": FlagArgTypeNumber, "--match": FlagArgTypeString,
			"--number-topics": FlagArgTypeString, "--order": FlagArgTypeString, "--owner": FlagArgTypeString,
			"--size": FlagArgTypeString, "--sort": FlagArgTypeString, "--stars": FlagArgTypeString,
			"--topic": FlagArgTypeString, "--updated": FlagArgTypeString, "--visibility": FlagArgTypeString,
		},
	},
	"gh search issues": {
		SafeFlags: map[string]FlagArgType{
			"--app": FlagArgTypeString, "--assignee": FlagArgTypeString, "--author": FlagArgTypeString,
			"--closed": FlagArgTypeString, "--commenter": FlagArgTypeString, "--comments": FlagArgTypeString,
			"--created": FlagArgTypeString, "--include-prs": FlagArgTypeNone, "--interactions": FlagArgTypeString,
			"--involves": FlagArgTypeString, "--json": FlagArgTypeString, "--label": FlagArgTypeString,
			"--language": FlagArgTypeString, "--limit": FlagArgTypeNumber, "-L": FlagArgTypeNumber,
			"--locked": FlagArgTypeNone, "--match": FlagArgTypeString, "--mentions": FlagArgTypeString,
			"--milestone": FlagArgTypeString, "--no-assignee": FlagArgTypeNone, "--no-label": FlagArgTypeNone,
			"--no-milestone": FlagArgTypeNone, "--no-project": FlagArgTypeNone, "--order": FlagArgTypeString,
			"--owner": FlagArgTypeString, "--project": FlagArgTypeString, "--reactions": FlagArgTypeString,
			"--repo": FlagArgTypeString, "-R": FlagArgTypeString, "--sort": FlagArgTypeString,
			"--state": FlagArgTypeString, "--team-mentions": FlagArgTypeString, "--updated": FlagArgTypeString,
			"--visibility": FlagArgTypeString,
		},
	},
	"gh search prs": {
		SafeFlags: map[string]FlagArgType{
			"--app": FlagArgTypeString, "--assignee": FlagArgTypeString, "--author": FlagArgTypeString,
			"--base": FlagArgTypeString, "-B": FlagArgTypeString, "--checks": FlagArgTypeString,
			"--closed": FlagArgTypeString, "--commenter": FlagArgTypeString, "--comments": FlagArgTypeString,
			"--created": FlagArgTypeString, "--draft": FlagArgTypeNone, "--head": FlagArgTypeString,
			"-H": FlagArgTypeString, "--interactions": FlagArgTypeString, "--involves": FlagArgTypeString,
			"--json": FlagArgTypeString, "--label": FlagArgTypeString, "--language": FlagArgTypeString,
			"--limit": FlagArgTypeNumber, "-L": FlagArgTypeNumber, "--locked": FlagArgTypeNone,
			"--match": FlagArgTypeString, "--mentions": FlagArgTypeString, "--merged": FlagArgTypeNone,
			"--merged-at": FlagArgTypeString, "--milestone": FlagArgTypeString,
			"--no-assignee": FlagArgTypeNone, "--no-label": FlagArgTypeNone,
			"--no-milestone": FlagArgTypeNone, "--no-project": FlagArgTypeNone,
			"--order": FlagArgTypeString, "--owner": FlagArgTypeString, "--project": FlagArgTypeString,
			"--reactions": FlagArgTypeString, "--repo": FlagArgTypeString, "-R": FlagArgTypeString,
			"--review": FlagArgTypeString, "--review-requested": FlagArgTypeString,
			"--reviewed-by": FlagArgTypeString, "--sort": FlagArgTypeString, "--state": FlagArgTypeString,
			"--team-mentions": FlagArgTypeString, "--updated": FlagArgTypeString,
			"--visibility": FlagArgTypeString,
		},
	},
	"gh search commits": {
		SafeFlags: map[string]FlagArgType{
			"--author": FlagArgTypeString, "--author-date": FlagArgTypeString,
			"--author-email": FlagArgTypeString, "--author-name": FlagArgTypeString,
			"--committer": FlagArgTypeString, "--committer-date": FlagArgTypeString,
			"--committer-email": FlagArgTypeString, "--committer-name": FlagArgTypeString,
			"--hash": FlagArgTypeString, "--json": FlagArgTypeString,
			"--limit": FlagArgTypeNumber, "-L": FlagArgTypeNumber,
			"--merge": FlagArgTypeNone, "--order": FlagArgTypeString, "--owner": FlagArgTypeString,
			"--parent": FlagArgTypeString, "--repo": FlagArgTypeString, "-R": FlagArgTypeString,
			"--sort": FlagArgTypeString, "--tree": FlagArgTypeString, "--visibility": FlagArgTypeString,
		},
	},
	"gh search code": {
		SafeFlags: map[string]FlagArgType{
			"--extension": FlagArgTypeString, "--filename": FlagArgTypeString,
			"--json": FlagArgTypeString, "--language": FlagArgTypeString,
			"--limit": FlagArgTypeNumber, "-L": FlagArgTypeNumber, "--match": FlagArgTypeString,
			"--owner": FlagArgTypeString, "--repo": FlagArgTypeString, "-R": FlagArgTypeString,
			"--size": FlagArgTypeString,
		},
	},
}

// DockerReadOnlyCommands 는 docker 읽기 전용 명령 맵이다.
var DockerReadOnlyCommands = map[string]ExternalCommandConfig{
	"docker logs": {
		SafeFlags: map[string]FlagArgType{
			"--follow": FlagArgTypeNone, "-f": FlagArgTypeNone,
			"--tail": FlagArgTypeString, "-n": FlagArgTypeString,
			"--timestamps": FlagArgTypeNone, "-t": FlagArgTypeNone,
			"--since": FlagArgTypeString, "--until": FlagArgTypeString, "--details": FlagArgTypeNone,
		},
	},
	"docker inspect": {
		SafeFlags: map[string]FlagArgType{
			"--format": FlagArgTypeString, "-f": FlagArgTypeString,
			"--type": FlagArgTypeString, "--size": FlagArgTypeNone, "-s": FlagArgTypeNone,
		},
	},
}

// RipgrepReadOnlyCommands 는 rg (ripgrep) 읽기 전용 검색 명령 맵이다.
var RipgrepReadOnlyCommands = map[string]ExternalCommandConfig{
	"rg": {
		SafeFlags: map[string]FlagArgType{
			"-e": FlagArgTypeString, "--regexp": FlagArgTypeString, "-f": FlagArgTypeString,
			"-i": FlagArgTypeNone, "--ignore-case": FlagArgTypeNone,
			"-S": FlagArgTypeNone, "--smart-case": FlagArgTypeNone,
			"-F": FlagArgTypeNone, "--fixed-strings": FlagArgTypeNone,
			"-w": FlagArgTypeNone, "--word-regexp": FlagArgTypeNone,
			"-v": FlagArgTypeNone, "--invert-match": FlagArgTypeNone,
			"-c": FlagArgTypeNone, "--count": FlagArgTypeNone,
			"-l": FlagArgTypeNone, "--files-with-matches": FlagArgTypeNone,
			"--files-without-match": FlagArgTypeNone,
			"-n":                    FlagArgTypeNone, "--line-number": FlagArgTypeNone,
			"-o": FlagArgTypeNone, "--only-matching": FlagArgTypeNone,
			"-A": FlagArgTypeNumber, "--after-context": FlagArgTypeNumber,
			"-B": FlagArgTypeNumber, "--before-context": FlagArgTypeNumber,
			"-C": FlagArgTypeNumber, "--context": FlagArgTypeNumber,
			"-H": FlagArgTypeNone, "-h": FlagArgTypeNone,
			"--heading": FlagArgTypeNone, "--no-heading": FlagArgTypeNone,
			"-q": FlagArgTypeNone, "--quiet": FlagArgTypeNone, "--column": FlagArgTypeNone,
			"-g": FlagArgTypeString, "--glob": FlagArgTypeString,
			"-t": FlagArgTypeString, "--type": FlagArgTypeString,
			"-T": FlagArgTypeString, "--type-not": FlagArgTypeString, "--type-list": FlagArgTypeNone,
			"--hidden": FlagArgTypeNone, "--no-ignore": FlagArgTypeNone, "-u": FlagArgTypeNone,
			"-m": FlagArgTypeNumber, "--max-count": FlagArgTypeNumber,
			"-d": FlagArgTypeNumber, "--max-depth": FlagArgTypeNumber,
			"-a": FlagArgTypeNone, "--text": FlagArgTypeNone,
			"-z": FlagArgTypeNone, "-L": FlagArgTypeNone, "--follow": FlagArgTypeNone,
			"--color": FlagArgTypeString, "--json": FlagArgTypeNone, "--stats": FlagArgTypeNone,
			"--help": FlagArgTypeNone, "--version": FlagArgTypeNone, "--debug": FlagArgTypeNone,
			"--": FlagArgTypeNone,
		},
	},
}

// PyrightReadOnlyCommands 는 pyright 정적 타입 체커 읽기 전용 명령 맵이다.
var PyrightReadOnlyCommands = map[string]ExternalCommandConfig{
	"pyright": {
		RespectsDoubleDash: boolPtr(false),
		SafeFlags: map[string]FlagArgType{
			"--outputjson": FlagArgTypeNone, "--project": FlagArgTypeString, "-p": FlagArgTypeString,
			"--pythonversion": FlagArgTypeString, "--pythonplatform": FlagArgTypeString,
			"--typeshedpath": FlagArgTypeString, "--venvpath": FlagArgTypeString,
			"--level": FlagArgTypeString, "--stats": FlagArgTypeNone,
			"--verbose": FlagArgTypeNone, "--version": FlagArgTypeNone,
			"--dependencies": FlagArgTypeNone, "--warnings": FlagArgTypeNone,
		},
		AdditionalCommandIsDangerous: func(_ string, args []string) bool {
			for _, t := range args {
				if t == "--watch" || t == "-w" {
					return true
				}
			}
			return false
		},
	},
}

// ExternalReadonlyCommandNames 는 bash/powershell 양쪽에서 동일하게 동작하는
// 크로스 플랫폼 읽기 전용 외부 도구 이름 목록이다.
var ExternalReadonlyCommandNames = []string{
	"docker ps",
	"docker images",
}
