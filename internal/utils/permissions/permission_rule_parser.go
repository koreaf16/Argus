// Package permissions — 권한 규칙 문자열 파서.
//
// 파일 역할: "ToolName(content)" 형식의 권한 규칙 문자열을 파싱하고
//
//	직렬화한다. 괄호 이스케이프를 처리하며 레거시 도구 이름을 정규화한다.
//
// 포함 모듈:
//   - EscapeRuleContent() / UnescapeRuleContent(): 괄호 이스케이프 처리.
//   - PermissionRuleValueFromString(): 문자열 → PermissionRuleValue.
//   - PermissionRuleValueToString(): PermissionRuleValue → 문자열.
//   - NormalizeLegacyToolName(): 레거시 도구 이름 → 현재 이름.
//
// 호출/사용 방식:
//   - internal/utils/permissions/permissions_loader.go 의 설정 파싱 시 참조.
//
// 연결:
//   - 원본: src/utils/permissions/permissionRuleParser.ts
//   - internal/types/permissions.go: PermissionRuleValue
package permissions

import (
	"strings"

	"github.com/koreaf16/argus/internal/types"
)

// legacyToolNameAliases 는 레거시 도구 이름 → 현재 정규 이름 매핑이다.
var legacyToolNameAliases = map[string]string{
	"Task":            "Agent",
	"KillShell":       "TaskStop",
	"AgentOutputTool": "TaskOutput",
	"BashOutputTool":  "TaskOutput",
}

// NormalizeLegacyToolName 은 레거시 도구 이름을 현재 정규 이름으로 변환한다.
func NormalizeLegacyToolName(name string) string {
	if canonical, ok := legacyToolNameAliases[name]; ok {
		return canonical
	}
	return name
}

// GetLegacyToolNames 는 정규 이름에 매핑된 레거시 이름 목록을 반환한다.
func GetLegacyToolNames(canonicalName string) []string {
	var result []string
	for legacy, canonical := range legacyToolNameAliases {
		if canonical == canonicalName {
			result = append(result, legacy)
		}
	}
	return result
}

// EscapeRuleContent 는 규칙 내용의 역슬래시·괄호를 이스케이프한다.
// 이스케이프 순서: 역슬래시 → 괄호.
func EscapeRuleContent(content string) string {
	s := strings.ReplaceAll(content, `\`, `\\`)
	s = strings.ReplaceAll(s, `(`, `\(`)
	s = strings.ReplaceAll(s, `)`, `\)`)
	return s
}

// UnescapeRuleContent 는 EscapeRuleContent 의 역연산이다.
// 이스케이프 해제 순서: 괄호 → 역슬래시.
func UnescapeRuleContent(content string) string {
	s := strings.ReplaceAll(content, `\(`, `(`)
	s = strings.ReplaceAll(s, `\)`, `)`)
	s = strings.ReplaceAll(s, `\\`, `\`)
	return s
}

// findFirstUnescapedChar 는 str 에서 char 의 첫 번째 비이스케이프 위치를 반환한다.
// 없으면 -1 반환.
func findFirstUnescapedChar(str string, char byte) int {
	for i := 0; i < len(str); i++ {
		if str[i] == char {
			count := 0
			for j := i - 1; j >= 0 && str[j] == '\\'; j-- {
				count++
			}
			if count%2 == 0 {
				return i
			}
		}
	}
	return -1
}

// findLastUnescapedChar 는 str 에서 char 의 마지막 비이스케이프 위치를 반환한다.
// 없으면 -1 반환.
func findLastUnescapedChar(str string, char byte) int {
	for i := len(str) - 1; i >= 0; i-- {
		if str[i] == char {
			count := 0
			for j := i - 1; j >= 0 && str[j] == '\\'; j-- {
				count++
			}
			if count%2 == 0 {
				return i
			}
		}
	}
	return -1
}

// PermissionRuleValueFromString 은 "ToolName" 또는 "ToolName(content)" 형식의
// 규칙 문자열을 파싱하여 PermissionRuleValue 를 반환한다.
func PermissionRuleValueFromString(ruleString string) types.PermissionRuleValue {
	openIdx := findFirstUnescapedChar(ruleString, '(')
	if openIdx == -1 {
		return types.PermissionRuleValue{ToolName: NormalizeLegacyToolName(ruleString)}
	}

	closeIdx := findLastUnescapedChar(ruleString, ')')
	if closeIdx == -1 || closeIdx <= openIdx {
		return types.PermissionRuleValue{ToolName: NormalizeLegacyToolName(ruleString)}
	}

	if closeIdx != len(ruleString)-1 {
		return types.PermissionRuleValue{ToolName: NormalizeLegacyToolName(ruleString)}
	}

	toolName := ruleString[:openIdx]
	rawContent := ruleString[openIdx+1 : closeIdx]

	if toolName == "" {
		return types.PermissionRuleValue{ToolName: NormalizeLegacyToolName(ruleString)}
	}

	if rawContent == "" || rawContent == "*" {
		return types.PermissionRuleValue{ToolName: NormalizeLegacyToolName(toolName)}
	}

	return types.PermissionRuleValue{
		ToolName:    NormalizeLegacyToolName(toolName),
		RuleContent: UnescapeRuleContent(rawContent),
	}
}

// PermissionRuleValueToString 은 PermissionRuleValue 를 규칙 문자열로 직렬화한다.
func PermissionRuleValueToString(rv types.PermissionRuleValue) string {
	if rv.RuleContent == "" {
		return rv.ToolName
	}
	return rv.ToolName + "(" + EscapeRuleContent(rv.RuleContent) + ")"
}
