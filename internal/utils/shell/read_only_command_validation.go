// Package shell — 읽기 전용 명령 플래그 검증 공통 타입 및 함수.
//
// 파일 역할: FlagArgType, ExternalCommandConfig 타입을 정의하고, ValidateFlags 와
//
//	ContainsVulnerableUNCPath 함수를 구현한다.
//	BashTool / PowerShellTool 에서 공유하는 핵심 보안 검증 로직이다.
//
// 포함 모듈:
//   - FlagArgType: 플래그 인수 타입 (none/number/string/char/{}/EOF)
//   - ExternalCommandConfig: 명령별 안전 플래그 설정
//   - ValidateFlags: 토큰화된 인수 목록 검증
//   - ContainsVulnerableUNCPath: UNC 경로 취약점 탐지
//
// 호출/사용 방식:
//   - internal/tools/bash/bash_tool.go 에서 isCommandSafeViaFlagParsing 호출 체인
//   - read_only_git_commands.go / read_only_gh_commands.go 에서 config 정의에 사용
//
// 연결:
//   - 원본: src/utils/shell/readOnlyCommandValidation.ts
package shell

import (
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

// FlagArgType 은 플래그가 받을 수 있는 인수의 종류를 나타낸다.
type FlagArgType string

const (
	FlagArgTypeNone   FlagArgType = "none"   // 인수 없음 (--color, -n)
	FlagArgTypeNumber FlagArgType = "number" // 정수 (--context=3)
	FlagArgTypeString FlagArgType = "string" // 임의 문자열
	FlagArgTypeChar   FlagArgType = "char"   // 단일 문자
	FlagArgTypeBrace  FlagArgType = "{}"     // 리터럴 "{}" 만 허용
	FlagArgTypeEOF    FlagArgType = "EOF"    // 리터럴 "EOF" 만 허용
)

// ExternalCommandConfig 는 명령별 안전 플래그와 추가 위험 검사를 담는다.
type ExternalCommandConfig struct {
	// SafeFlags 는 플래그 이름 → 인수 타입 매핑이다.
	SafeFlags map[string]FlagArgType
	// AdditionalCommandIsDangerous 는 추가적인 위험 여부를 검사하는 콜백이다.
	// true 를 반환하면 해당 명령은 위험한 것으로 간주된다.
	AdditionalCommandIsDangerous func(rawCommand string, args []string) bool
	// RespectsDoubleDash 가 false 이면 `--` 를 end-of-options 로 처리하지 않는다.
	// 기본값은 true (대부분의 도구가 POSIX `--` 를 준수함).
	RespectsDoubleDash *bool
}

// ValidateFlagsOptions 는 ValidateFlags 에 전달하는 옵션이다.
type ValidateFlagsOptions struct {
	CommandName         string
	XargsTargetCommands []string
}

// flagPattern 은 유효한 플래그 이름 패턴이다 (-x 또는 --xxx).
var flagPattern = regexp.MustCompile(`^-[a-zA-Z0-9_-]`)

// ValidateFlags 는 토큰 목록을 config 에 정의된 안전 플래그 규칙에 따라 검증한다.
// 모든 플래그가 안전하면 true, 하나라도 미등록/잘못된 플래그가 있으면 false 를 반환한다.
func ValidateFlags(config ExternalCommandConfig, tokens []string, opts ValidateFlagsOptions) bool {
	respectsDoubleDash := true
	if config.RespectsDoubleDash != nil {
		respectsDoubleDash = *config.RespectsDoubleDash
	}

	i := 0
	for i < len(tokens) {
		token := tokens[i]
		if token == "" {
			i++
			continue
		}

		// xargs 특수 처리: 타겟 명령을 만나면 검증 중단
		if opts.XargsTargetCommands != nil && opts.CommandName == "xargs" &&
			(!strings.HasPrefix(token, "-") || token == "--") {
			if token == "--" && i+1 < len(tokens) {
				i++
				token = tokens[i]
			}
			if slices.Contains(opts.XargsTargetCommands, token) {
				return true
			}
			return false
		}

		// `--` end-of-options 처리
		if token == "--" {
			if respectsDoubleDash {
				// -- 이후는 모두 positional 인수 → 허용
				return true
			}
			i++
			continue
		}

		// 플래그인지 확인
		if strings.HasPrefix(token, "-") && len(token) > 1 && flagPattern.MatchString(token) {
			flag, inlineValue, hasEquals := strings.Cut(token, "=")
			if !hasEquals {
				flag = token
			}

			if flag == "" {
				return false
			}

			flagArgType, known := config.SafeFlags[flag]

			if !known {
				// git -<number> 단축 형식 (예: git log -5)
				if opts.CommandName == "git" && regexp.MustCompile(`^-\d+$`).MatchString(flag) {
					i++
					continue
				}

				// grep/rg 의 -A20 형식 (숫자 부착)
				if (opts.CommandName == "grep" || opts.CommandName == "rg") &&
					strings.HasPrefix(flag, "-") && !strings.HasPrefix(flag, "--") && len(flag) > 2 {
					potentialFlag := flag[:2]
					potentialValue := flag[2:]
					if pt, ok := config.SafeFlags[potentialFlag]; ok && regexp.MustCompile(`^\d+$`).MatchString(potentialValue) {
						if pt == FlagArgTypeNumber || pt == FlagArgTypeString {
							if validateFlagArgument(potentialValue, pt) {
								i++
								continue
							}
							return false
						}
					}
				}

				// 복합 단일 문자 플래그 (-nr)
				if strings.HasPrefix(flag, "-") && !strings.HasPrefix(flag, "--") && len(flag) > 2 {
					allNone := true
					for j := 1; j < len(flag); j++ {
						sf := "-" + string(flag[j])
						st, ok := config.SafeFlags[sf]
						if !ok {
							return false
						}
						if st != FlagArgTypeNone {
							allNone = false
							break
						}
					}
					if !allNone {
						return false
					}
					i++
					continue
				}

				return false // 알 수 없는 플래그
			}

			// 플래그 인수 처리
			if flagArgType == FlagArgTypeNone {
				if hasEquals {
					return false // none 타입은 값 불허
				}
				i++
			} else {
				var argValue string
				if hasEquals {
					argValue = inlineValue
					i++
				} else {
					// 다음 토큰이 인수
					if i+1 >= len(tokens) {
						return false
					}
					next := tokens[i+1]
					if next != "" && strings.HasPrefix(next, "-") && len(next) > 1 && flagPattern.MatchString(next) {
						return false // 다음 토큰도 플래그 → 인수 누락
					}
					argValue = tokens[i+1]
					i += 2
				}

				// string 타입은 '-' 로 시작하는 값 거부 (단, git --sort 예외)
				if flagArgType == FlagArgTypeString && strings.HasPrefix(argValue, "-") {
					if flag == "--sort" && opts.CommandName == "git" &&
						regexp.MustCompile(`^-[a-zA-Z]`).MatchString(argValue) {
						// git --sort=-refname 형태 허용
					} else {
						return false
					}
				}

				if !validateFlagArgument(argValue, flagArgType) {
					return false
				}
			}
		} else {
			// 비플래그 positional 인수 → 허용
			i++
		}
	}
	return true
}

// validateFlagArgument 는 플래그 인수 값이 지정된 타입과 일치하는지 검사한다.
func validateFlagArgument(value string, argType FlagArgType) bool {
	switch argType {
	case FlagArgTypeNone:
		return false // none 타입에서는 호출되지 않아야 함
	case FlagArgTypeNumber:
		_, err := strconv.Atoi(value)
		return err == nil && regexp.MustCompile(`^\d+$`).MatchString(value)
	case FlagArgTypeString:
		return true
	case FlagArgTypeChar:
		return len(value) == 1
	case FlagArgTypeBrace:
		return value == "{}"
	case FlagArgTypeEOF:
		return value == "EOF"
	default:
		return false
	}
}

// uncBackslash 는 백슬래시 UNC 경로 패턴이다.
var uncBackslash = regexp.MustCompile(`(?i)\\\\[^\s\\/]+(?:@(?:\d+|ssl))?(?:[\\/]|$|\s)`)

// uncForwardSlash 는 슬래시 UNC 경로 패턴이다 (URL 제외).
var uncForwardSlash = regexp.MustCompile(`(?i)(?:^|[^:])//[^\s\\/]+(?:@(?:\d+|ssl))?(?:[\\/]|$|\s)`)

// uncMixedFwdBack 는 / 뒤에 2개 이상의 \ 가 오는 패턴이다.
var uncMixedFwdBack = regexp.MustCompile(`/\\{2,}[^\s\\/]`)

// uncMixedBackFwd 는 2개 이상의 \ 뒤에 / 가 오는 패턴이다.
var uncMixedBackFwd = regexp.MustCompile(`\\{2,}/[^\s\\/]`)

// uncWebDAVSSL 은 WebDAV SSL 포트 패턴이다.
var uncWebDAVSSL = regexp.MustCompile(`(?i)@SSL@\d+|@\d+@SSL`)

// uncDavWWWRoot 는 WebDAV DavWWWRoot 패턴이다.
var uncDavWWWRoot = regexp.MustCompile(`(?i)DavWWWRoot`)

// uncIPv4Back 는 IPv4 백슬래시 UNC 패턴이다.
var uncIPv4Back = regexp.MustCompile(`^\\\\(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})[\\/]`)

// uncIPv4Fwd 는 IPv4 슬래시 UNC 패턴이다.
var uncIPv4Fwd = regexp.MustCompile(`^//(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})[\\/]`)

// uncIPv6Back 는 IPv6 백슬래시 UNC 패턴이다.
var uncIPv6Back = regexp.MustCompile(`^\\\\(\[[\da-fA-F:]+\])[\\/]`)

// uncIPv6Fwd 는 IPv6 슬래시 UNC 패턴이다.
var uncIPv6Fwd = regexp.MustCompile(`^//(\[[\da-fA-F:]+\])[\\/]`)

// ContainsVulnerableUNCPath 는 명령 또는 경로에 UNC 경로가 포함되어 있는지 검사한다.
// Windows 에서만 true 를 반환할 수 있으며, 다른 플랫폼에서는 항상 false 를 반환한다.
func ContainsVulnerableUNCPath(command string) bool {
	if runtime.GOOS != "windows" {
		return false
	}

	if uncBackslash.MatchString(command) {
		return true
	}
	if uncForwardSlash.MatchString(command) {
		return true
	}
	if uncMixedFwdBack.MatchString(command) {
		return true
	}
	if uncMixedBackFwd.MatchString(command) {
		return true
	}
	if uncWebDAVSSL.MatchString(command) {
		return true
	}
	if uncDavWWWRoot.MatchString(command) {
		return true
	}
	if uncIPv4Back.MatchString(command) || uncIPv4Fwd.MatchString(command) {
		return true
	}
	if uncIPv6Back.MatchString(command) || uncIPv6Fwd.MatchString(command) {
		return true
	}
	return false
}
