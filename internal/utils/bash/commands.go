// Package bash — 셸 명령 분리·분석 유틸리티.
//
// 파일 역할: 복합 명령(&&, ||, ;, |)을 서브커맨드로 분리하고,
//
//	출력 리다이렉션을 추출하며, 도움말 명령·안전하지 않은 복합 명령을 감지한다.
//
// 포함 모듈:
//   - SplitCommandWithOperators: 연산자 기준으로 명령을 토큰 슬라이스로 분리
//   - SplitCommandDeprecated: 단순 세미콜론/AND/OR 기준 서브커맨드 분리 (레거시)
//   - FilterControlOperators: 연산자 토큰을 제거하고 명령 토큰만 반환
//   - IsHelpCommand: --help / -h 플래그를 포함하는지 확인
//   - IsUnsafeCompoundCommandDeprecated: 위험한 복합 명령 감지 (레거시)
//   - ExtractOutputRedirections: 명령에서 출력 리다이렉션을 추출
//
// 호출/사용 방식:
//   - internal/utils/permissions/bash_classifier.go 가 복합 명령 분석에 사용
//   - internal/utils/bash/prefix.go 가 서브커맨드 분리에 사용
//
// 연결:
//   - 원본: src/utils/bash/commands.ts
package bash

import "strings"

// ControlOperators 는 셸 제어 연산자 집합이다.
var ControlOperators = map[string]bool{
	"&&": true, "||": true, ";": true, "|": true, "&": true,
}

// SplitCommandWithOperators 는 command 를 연산자와 명령 토큰으로 분리한다.
// 연산자 토큰(&&, ||, ;, |)이 포함된 슬라이스를 반환한다.
func SplitCommandWithOperators(command string) []string {
	tokens := tokenizeSimple(command)
	var result []string
	for _, tok := range tokens {
		if ControlOperators[tok] {
			result = append(result, tok)
		} else {
			result = append(result, tok)
		}
	}
	return result
}

// FilterControlOperators 는 연산자 토큰을 제거하고 명령 토큰만 반환한다.
func FilterControlOperators(tokens []string) []string {
	var result []string
	for _, tok := range tokens {
		if !ControlOperators[tok] {
			result = append(result, tok)
		}
	}
	return result
}

// SplitCommandDeprecated 는 &&, ||, ; 기준으로 command 를 서브커맨드 슬라이스로 분리한다.
// 레거시 코드에서 사용한다. 인용 부호를 완전히 존중하지 않는다.
func SplitCommandDeprecated(command string) []string {
	// 간단한 구분자 기반 분리 (레거시)
	var parts []string
	current := strings.Builder{}
	for i := 0; i < len(command); i++ {
		ch := command[i]
		// &&, ||
		if (ch == '&' || ch == '|') && i+1 < len(command) && command[i+1] == ch {
			if current.Len() > 0 {
				parts = append(parts, strings.TrimSpace(current.String()))
				current.Reset()
			}
			i++ // 두 번째 문자 건너뜀
			continue
		}
		if ch == ';' {
			if current.Len() > 0 {
				parts = append(parts, strings.TrimSpace(current.String()))
				current.Reset()
			}
			continue
		}
		current.WriteByte(ch)
	}
	if current.Len() > 0 {
		if s := strings.TrimSpace(current.String()); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return []string{command}
	}
	return parts
}

// IsHelpCommand 는 command 가 --help 또는 -h 플래그를 포함하는지 반환한다.
func IsHelpCommand(command string) bool {
	tokens := tokenizeSimple(command)
	for _, tok := range tokens {
		if tok == "--help" || tok == "-h" {
			return true
		}
	}
	return false
}

// IsUnsafeCompoundCommandDeprecated 는 위험한 복합 명령 패턴을 감지한다 (레거시).
func IsUnsafeCompoundCommandDeprecated(command string) bool {
	subs := SplitCommandDeprecated(command)
	return len(subs) > 1
}

// ExtractOutputRedirectionsResult 는 리다이렉션 추출 결과다.
type ExtractOutputRedirectionsResult struct {
	CommandWithoutRedirections string
	Redirections               []OutputRedirection
}

// ExtractOutputRedirections 는 command 에서 '>' 와 '>>' 리다이렉션을 추출한다.
func ExtractOutputRedirections(command string) ExtractOutputRedirectionsResult {
	cmd, redirections := extractOutputRedirections(command)
	return ExtractOutputRedirectionsResult{
		CommandWithoutRedirections: cmd,
		Redirections:               redirections,
	}
}
