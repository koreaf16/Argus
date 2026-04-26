// Package permissions — 셸 도구 권한 규칙 매칭 유틸리티.
//
// 파일 역할: Bash/PowerShell 명령을 권한 규칙에 매칭하는 로직.
//
//	exact(완전 일치), prefix(접두사), wildcard(와일드카드) 세 가지 규칙 타입을 지원한다.
//
// 포함 모듈:
//   - ShellPermissionRule: 파싱된 규칙 유니언 타입.
//   - ParsePermissionRule(): 문자열 → ShellPermissionRule.
//   - MatchesPermissionRule(): 명령 → 규칙 매칭 여부.
//   - MatchWildcardPattern(): 와일드카드 패턴 매칭.
//   - SuggestionForExactCommand() / SuggestionForPrefix(): 제안 생성.
//
// 호출/사용 방식:
//   - internal/utils/permissions/permissions.go 에서 규칙 매칭 시 참조.
//
// 연결:
//   - 원본: src/utils/permissions/shellRuleMatching.ts
//   - internal/types/permissions.go: PermissionUpdate
package permissions

import (
	"regexp"
	"strings"

	"github.com/koreaf16/argus/internal/types"
)

// ShellPermissionRuleType 은 파싱된 규칙의 종류다.
type ShellPermissionRuleType string

const (
	RuleTypeExact    ShellPermissionRuleType = "exact"
	RuleTypePrefix   ShellPermissionRuleType = "prefix"
	RuleTypeWildcard ShellPermissionRuleType = "wildcard"
)

// ShellPermissionRule 은 파싱된 권한 규칙이다.
type ShellPermissionRule struct {
	Type    ShellPermissionRuleType
	Command string // exact 타입
	Prefix  string // prefix 타입
	Pattern string // wildcard 타입
}

// permissionRuleExtractPrefix 는 레거시 ":*" 접두사 구문을 파싱한다 (예: "npm:*" → "npm").
func permissionRuleExtractPrefix(rule string) (string, bool) {
	if strings.HasSuffix(rule, ":*") {
		return rule[:len(rule)-2], true
	}
	return "", false
}

// hasWildcards 는 패턴에 비이스케이프 와일드카드가 있는지 확인한다.
// 레거시 ":*" 구문은 와일드카드로 취급하지 않는다.
func hasWildcards(pattern string) bool {
	if strings.HasSuffix(pattern, ":*") {
		return false
	}
	for i, ch := range pattern {
		if ch == '*' {
			count := 0
			for j := i - 1; j >= 0 && pattern[j] == '\\'; j-- {
				count++
			}
			if count%2 == 0 {
				return true
			}
		}
	}
	return false
}

// ParsePermissionRule 은 규칙 문자열을 ShellPermissionRule 로 파싱한다.
func ParsePermissionRule(rule string) ShellPermissionRule {
	if prefix, ok := permissionRuleExtractPrefix(rule); ok {
		return ShellPermissionRule{Type: RuleTypePrefix, Prefix: prefix}
	}
	if hasWildcards(rule) {
		return ShellPermissionRule{Type: RuleTypeWildcard, Pattern: rule}
	}
	return ShellPermissionRule{Type: RuleTypeExact, Command: rule}
}

// MatchWildcardPattern 은 와일드카드 패턴이 command 와 일치하는지 검사한다.
// '*' 는 임의 문자열에 매칭, '\*' 는 리터럴 '*', '\\' 는 리터럴 '\'.
func MatchWildcardPattern(pattern, command string, caseInsensitive bool) bool {
	trimmed := strings.TrimSpace(pattern)

	// 이스케이프된 시퀀스를 플레이스홀더로 치환
	const escapedStarPH = "\x00ESCAPED_STAR\x00"
	const escapedBackslashPH = "\x00ESCAPED_BACKSLASH\x00"

	processed := strings.Builder{}
	i := 0
	for i < len(trimmed) {
		if trimmed[i] == '\\' && i+1 < len(trimmed) {
			switch trimmed[i+1] {
			case '*':
				processed.WriteString(escapedStarPH)
				i += 2
				continue
			case '\\':
				processed.WriteString(escapedBackslashPH)
				i += 2
				continue
			}
		}
		processed.WriteByte(trimmed[i])
		i++
	}

	// 정규식 특수문자 이스케이프 (단 '*' 제외)
	escaped := regexp.QuoteMeta(processed.String())
	// QuoteMeta 가 '*' 를 '\*' 로 만들지 않으므로 직접 처리
	escaped = strings.ReplaceAll(escaped, `\*`, `.*`)

	// 플레이스홀더 복원
	regexPattern := strings.ReplaceAll(escaped, escapedStarPH, `\*`)
	regexPattern = strings.ReplaceAll(regexPattern, escapedBackslashPH, `\\`)

	// 단일 와일드카드로 끝나는 " *" 패턴은 인수 없이도 매칭 허용
	unescapedStarCount := strings.Count(processed.String(), "*")
	if strings.HasSuffix(regexPattern, ` .*`) && unescapedStarCount == 1 {
		regexPattern = regexPattern[:len(regexPattern)-3] + `( .*)?`
	}

	flags := "(?s)"
	if caseInsensitive {
		flags = "(?si)"
	}
	re, err := regexp.Compile(flags + "^" + regexPattern + "$")
	if err != nil {
		return false
	}
	return re.MatchString(command)
}

// MatchesPermissionRule 은 command 가 규칙에 매칭되는지 확인한다.
func MatchesPermissionRule(rule ShellPermissionRule, command string, caseInsensitive bool) bool {
	switch rule.Type {
	case RuleTypeExact:
		if caseInsensitive {
			return strings.EqualFold(rule.Command, command)
		}
		return rule.Command == command
	case RuleTypePrefix:
		cmd := command
		pfx := rule.Prefix
		if caseInsensitive {
			cmd = strings.ToLower(cmd)
			pfx = strings.ToLower(pfx)
		}
		return cmd == pfx || strings.HasPrefix(cmd, pfx+" ")
	case RuleTypeWildcard:
		return MatchWildcardPattern(rule.Pattern, command, caseInsensitive)
	}
	return false
}

// SuggestionForExactCommand 는 완전 일치 허용 규칙 추가 제안을 반환한다.
func SuggestionForExactCommand(toolName, command string) []types.PermissionUpdate {
	return []types.PermissionUpdate{{
		Type:        types.UpdateTypeAddRules,
		Rules:       []types.PermissionRuleValue{{ToolName: toolName, RuleContent: command}},
		Behavior:    types.BehaviorAllow,
		Destination: types.UpdateDestLocalSettings,
	}}
}

// SuggestionForPrefix 는 접두사 허용 규칙 추가 제안을 반환한다.
func SuggestionForPrefix(toolName, prefix string) []types.PermissionUpdate {
	return []types.PermissionUpdate{{
		Type:        types.UpdateTypeAddRules,
		Rules:       []types.PermissionRuleValue{{ToolName: toolName, RuleContent: prefix + ":*"}},
		Behavior:    types.BehaviorAllow,
		Destination: types.UpdateDestLocalSettings,
	}}
}
