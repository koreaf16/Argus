// Package hooks
// File: matcher.go
// Description: Hook 매처 문법 파서 및 매칭 평가기
// Responsibility: "Bash(rm *)" 형식 패턴을 파싱하고 HookInput 과 매칭 여부를 판단

// Ported from: claude_cli/src/schemas/hooks.ts:16-27 (IfConditionSchema)
// 핵심 패턴: permission rule syntax — "ToolName(glob_pattern)" 으로 도구+입력 필터링.

package hooks

import (
	"path/filepath"
	"strings"
)

// pattern 은 파싱된 matcher 패턴이다.
type pattern struct {
	toolName string // 예: "Bash", "Write", "*"
	argGlob  string // 예: "rm *", "*.yaml", "" (arg 없으면 모든 입력 매칭)
}

// parseMatcher 는 "Bash(rm *)" 형식 문자열을 파싱한다.
// 괄호 없으면 toolName 만, 괄호 있으면 toolName + argGlob.
func parseMatcher(s string) pattern {
	s = strings.TrimSpace(s)
	open := strings.Index(s, "(")
	if open < 0 {
		return pattern{toolName: s}
	}
	close := strings.LastIndex(s, ")")
	if close < 0 || close < open {
		return pattern{toolName: s}
	}
	return pattern{
		toolName: strings.TrimSpace(s[:open]),
		argGlob:  strings.TrimSpace(s[open+1 : close]),
	}
}

// MatchInput 은 matchers 목록 중 하나라도 HookInput 과 매칭되는지 검사한다.
func MatchInput(matchers []MatcherDef, in HookInput) ([]HookDef, bool) {
	for _, m := range matchers {
		if matchSingle(m.Matcher, in) {
			return m.Hooks, true
		}
	}
	return nil, false
}

// matchSingle 은 단일 matcher 문자열이 HookInput 과 매칭되는지 판단한다.
func matchSingle(matcher string, in HookInput) bool {
	p := parseMatcher(matcher)

	// 도구 이름 매칭
	if p.toolName != "*" && !strings.EqualFold(p.toolName, in.Tool) {
		return false
	}

	// argGlob 없으면 도구 이름만으로 매칭
	if p.argGlob == "" {
		return true
	}

	// argGlob 매칭: 도구 입력에서 첫 번째 문자열 값 또는 "command" 키를 대상으로 한다.
	target := extractMatchTarget(in)
	matched, err := filepath.Match(p.argGlob, target)
	if err != nil {
		// 잘못된 glob 은 매칭 실패로 처리
		return false
	}
	return matched
}

// extractMatchTarget 은 HookInput 에서 glob 매칭 대상 문자열을 추출한다.
// Bash 도구: input["command"], Write/Read 도구: input["file_path"], 기타: 첫 문자열 값.
func extractMatchTarget(in HookInput) string {
	if in.Input == nil {
		return ""
	}
	// 우선순위: command → file_path → path → 첫 번째 string 값
	for _, key := range []string{"command", "file_path", "path"} {
		if v, ok := in.Input[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	for _, v := range in.Input {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
