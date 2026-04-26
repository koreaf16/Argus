// Package readonly
// File: registry.go
// Description: 읽기 전용 명령어 레지스트리 — 통합 조회 및 UNC 경로 검증
// Responsibility: IsReadOnly, Lookup, ContainsVulnerableUncPath 공개 API 제공

// Ported from: claude_cli/src/utils/shell/readOnlyCommandValidation.ts (validateFlags, containsVulnerableUncPath)

package readonly

import (
	"runtime"
	"strings"
)

// all 은 git/gh/docker/rg/posix 를 합친 전체 명령어 맵이다.
// 초기화는 패키지 로드 시 한 번만 수행한다.
var all map[string]CommandConfig

func init() {
	all = make(map[string]CommandConfig)
	for k, v := range GitReadOnlyCommands {
		all[k] = v
	}
	for k, v := range GhReadOnlyCommands {
		all[k] = v
	}
	for k, v := range DockerReadOnlyCommands {
		all[k] = v
	}
	for k, v := range RipgrepReadOnlyCommands {
		all[k] = v
	}
	for k, v := range PosixReadOnlyCommands {
		all[k] = v
	}
}

// Lookup 은 "git log", "ls" 등 명령어 키로 CommandConfig 를 반환한다.
func Lookup(cmdKey string) (CommandConfig, bool) {
	c, ok := all[cmdKey]
	return c, ok
}

// IsReadOnly 는 argv (공백 분리 전 원본 명령 또는 파싱된 토큰 목록)에서
// 명령어 전체가 읽기 전용인지 판정한다.
//
// 판정 순서:
//  1. argv[0] 이 복합 키("git log" 등)와 일치하는지 시도
//  2. argv[0] 단독 키("ls" 등)로 시도
//  3. 매칭 없으면 false (FAIL-CLOSED)
func IsReadOnly(argv []string) bool {
	if len(argv) == 0 {
		return false
	}

	var cfg CommandConfig
	var found bool
	var positionalStart int

	// 두 단어 복합 키 시도 (예: "git log")
	if len(argv) >= 2 {
		key2 := argv[0] + " " + argv[1]
		if c, ok := all[key2]; ok {
			cfg = c
			found = true
			positionalStart = 2
		}
	}
	// 단일 키 시도 (예: "ls")
	if !found {
		if c, ok := all[argv[0]]; ok {
			cfg = c
			found = true
			positionalStart = 1
		}
	}
	if !found {
		return false
	}

	return validateFlags(argv[positionalStart:], cfg)
}

// validateFlags 는 args (플래그+positional 목록)가 cfg 와 일치하는지 검사한다.
// Ported from: claude_cli/src/utils/shell/readOnlyCommandValidation.ts:validateFlags
func validateFlags(args []string, cfg CommandConfig) bool {
	respectDD := cfg.RespectsDoubleDash == nil || *cfg.RespectsDoubleDash
	var positionals []string
	seenDD := false

	i := 0
	for i < len(args) {
		token := args[i]

		if token == "--" && respectDD {
			seenDD = true
			i++
			continue
		}
		if seenDD || !strings.HasPrefix(token, "-") {
			positionals = append(positionals, token)
			i++
			continue
		}

		// --flag=value 형식 분리
		flagName := token
		var inlineVal string
		if idx := strings.Index(token, "="); idx >= 0 {
			flagName = token[:idx]
			inlineVal = token[idx+1:]
		}

		argType, ok := cfg.SafeFlags[flagName]
		if !ok {
			// 단일 문자 결합 플래그 처리: -la → -l + -a
			// 두 글자 이상이고 '-' 로 시작하며 '--' 가 아닌 경우
			if len(flagName) > 2 && flagName[0] == '-' && flagName[1] != '-' && inlineVal == "" {
				combined := true
				for _, ch := range flagName[1:] {
					short := "-" + string(ch)
					if t, ok2 := cfg.SafeFlags[short]; !ok2 || t != FlagArgNone {
						combined = false
						break
					}
				}
				if combined {
					i++
					continue
				}
			}
			return false // 미등록 플래그 → 위험 (FAIL-CLOSED)
		}

		switch argType {
		case FlagArgNone:
			if inlineVal != "" {
				return false // 인자를 받지 않는 플래그에 = 값 → 위험
			}
		case FlagArgNumber, FlagArgString, FlagArgChar:
			if inlineVal == "" {
				i++
				if i >= len(args) {
					return false // 인자 없음
				}
				// 다음 토큰이 플래그처럼 보이면 위험
				if strings.HasPrefix(args[i], "-") {
					return false
				}
			}
		case FlagArgLiteralBraces:
			if inlineVal == "" {
				i++
				if i >= len(args) || args[i] != "{}" {
					return false
				}
			} else if inlineVal != "{}" {
				return false
			}
		case FlagArgLiteralEOF:
			if inlineVal == "" {
				i++
				if i >= len(args) || args[i] != "EOF" {
					return false
				}
			} else if inlineVal != "EOF" {
				return false
			}
		}
		i++
	}

	// IsDangerous 콜백 검사
	if cfg.IsDangerous != nil && cfg.IsDangerous("", positionals) {
		return false
	}

	return true
}

// ContainsVulnerableUncPath 는 경로/명령 문자열에 UNC 경로가 포함되었는지 확인한다.
// Windows 에서만 검사하며, 자격증명 누출·WebDAV 공격을 방지한다.
// Ported from: claude_cli/src/utils/shell/readOnlyCommandValidation.ts:containsVulnerableUncPath
func ContainsVulnerableUncPath(s string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	return strings.Contains(s, `\\`) || strings.HasPrefix(s, `//`)
}
