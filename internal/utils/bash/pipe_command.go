// Package bash — 파이프 명령 재배치.
//
// 파일 역할: 파이프를 포함하는 명령을 eval 래핑 시 stdin 리다이렉션이
//
//	첫 번째 명령에 올바르게 적용되도록 재배치한다.
//
// 포함 모듈:
//   - RearrangePipeCommand: 파이프 명령의 stdin 리다이렉션 위치를 재배치
//
// 호출/사용 방식:
//   - internal/tools/bash/bash_tool.go 가 eval 래핑 전에 호출
//
// 연결:
//   - 원본: src/utils/bash/bashPipeCommand.ts
package bash

import (
	"regexp"
	"strings"
)

var (
	controlStructureRe = regexp.MustCompile(`\b(for|while|until|if|case|select)\s`)
	envVarRefRe        = regexp.MustCompile(`\$[A-Za-z_{]`)
)

// RearrangePipeCommand 는 파이프 명령을 eval 에 전달하기 위해 재배치한다.
// 파이프가 없거나 재배치할 수 없는 경우 그대로 single-quoted eval 인수로 감싼다.
func RearrangePipeCommand(command string) string {
	// backtick, $(), 셸 변수, 제어 구조, newline 이 있으면 단순 폴백
	if strings.Contains(command, "`") ||
		strings.Contains(command, "$(") ||
		envVarRefRe.MatchString(command) ||
		controlStructureRe.MatchString(command) ||
		strings.Contains(command, "\n") {
		return quoteForEvalWithStdinRedirect(command)
	}

	tokens := tokenizeSimple(command)
	if len(tokens) == 0 {
		return quoteForEvalWithStdinRedirect(command)
	}

	// 첫 번째 파이프 위치 탐색
	pipeIdx := -1
	for i, tok := range tokens {
		if tok == "|" {
			pipeIdx = i
			break
		}
	}

	if pipeIdx <= 0 {
		return quoteForEvalWithStdinRedirect(command)
	}

	// first_command < /dev/null | rest
	before := strings.Join(tokens[:pipeIdx], " ")
	after := strings.Join(tokens[pipeIdx:], " ")
	rebuilt := before + " < /dev/null " + after
	return singleQuoteForEval(rebuilt)
}

// quoteForEvalWithStdinRedirect 는 command 를 single-quote 로 감싸고 < /dev/null 을 붙인다.
func quoteForEvalWithStdinRedirect(command string) string {
	return singleQuoteForEval(command) + " < /dev/null"
}

// singleQuoteForEval 은 eval 인수용으로 s 를 단일 인용 부호로 감싼다.
// 내부의 단일 인용 부호는 '"'"' 패턴으로 이스케이프한다.
func singleQuoteForEval(s string) string {
	escaped := strings.ReplaceAll(s, "'", `'"'"'`)
	return "'" + escaped + "'"
}
