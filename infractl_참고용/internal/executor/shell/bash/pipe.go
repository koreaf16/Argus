// Package bash
// File: pipe.go
// Description: 파이프 명령어 stdin 재배치 — 비대화형 실행 보장
// Responsibility: RearrangePipe — 첫 파이프 세그먼트 앞에 < /dev/null 삽입

// Ported from: claude_cli/src/utils/bash/bashPipeCommand.ts:rearrangePipeCommand

package bash

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// RearrangePipe 는 파이프가 포함된 명령에서 첫 세그먼트의 stdin 을 /dev/null 로
// 리다이렉트해 비대화형 실행을 보장한다.
//
// 조건:
//   - 최상위 레벨에 파이프(|)가 있어야 한다.
//   - 첫 번째 파이프 세그먼트에 이미 stdin 리다이렉트(< / heredoc)가 없어야 한다.
//
// Ported from: claude_cli/src/utils/bash/bashPipeCommand.ts:rearrangePipeCommand
func RearrangePipe(src string, f *syntax.File) string {
	if !hasTopLevelPipe(f) {
		return src
	}
	if hasStdinRedirect(f) {
		return src
	}
	return "< /dev/null " + src
}

// hasTopLevelPipe 는 AST 최상위 레벨에 파이프 연산자가 있는지 확인한다.
func hasTopLevelPipe(f *syntax.File) bool {
	for _, stmt := range f.Stmts {
		if b, ok := stmt.Cmd.(*syntax.BinaryCmd); ok && b.Op == syntax.Pipe {
			return true
		}
	}
	return false
}

// hasStdinRedirect 는 첫 번째 최상위 Stmt 에 stdin 리다이렉트 또는 heredoc 이 있는지 확인한다.
func hasStdinRedirect(f *syntax.File) bool {
	if len(f.Stmts) == 0 {
		return false
	}
	stmt := f.Stmts[0]
	for _, rd := range stmt.Redirs {
		switch rd.Op {
		case syntax.RdrIn, syntax.Hdoc, syntax.DashHdoc:
			return true
		}
	}
	// BinaryCmd 의 X 측 stdin 리다이렉트 확인
	if b, ok := stmt.Cmd.(*syntax.BinaryCmd); ok && b.Op == syntax.Pipe {
		if b.X != nil {
			for _, rd := range b.X.Redirs {
				switch rd.Op {
				case syntax.RdrIn, syntax.Hdoc, syntax.DashHdoc:
					return true
				}
			}
		}
	}
	return false
}

// ContainsPipe 는 명령 문자열에 셸 파이프(|)가 포함되어 있는지 간단히 확인한다.
// AST 없이 빠른 pre-check 용도로만 사용한다.
func ContainsPipe(src string) bool {
	return strings.ContainsRune(src, '|')
}
