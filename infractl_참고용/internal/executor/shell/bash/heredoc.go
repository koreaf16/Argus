// Package bash
// File: heredoc.go
// Description: bash heredoc 정보 추출 — AST 기반
// Responsibility: *syntax.Redirect(Hdoc/DashHdoc) 노드에서 shell.Heredoc 메타데이터 수집

// Ported from: claude_cli/src/utils/bash/heredoc.ts
// TS 버전은 shell-quote 한계를 보완하기 위해 문자열 정규식으로 heredoc를 추출했지만,
// Go에서는 mvdan.cc/sh/v3 가 heredoc를 네이티브로 파싱하므로 AST 기반으로 처리한다.
// Heredoc 타입은 shell 패키지에 정의되어 있다 (임포트 순환 방지).

package bash

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/yourorg/infractl/internal/executor/shell"
)

// Extract 는 AST 에서 heredoc 목록을 추출한다.
// src 는 파싱에 사용된 원본 소스 문자열이며, 본문 오프셋 추출에 사용된다.
//
// Ported from: claude_cli/src/utils/bash/heredoc.ts:extractHeredocs
func Extract(f *syntax.File, src string) []shell.Heredoc {
	var result []shell.Heredoc

	syntax.Walk(f, func(n syntax.Node) bool {
		rd, ok := n.(*syntax.Redirect)
		if !ok {
			return true
		}
		if rd.Op != syntax.Hdoc && rd.Op != syntax.DashHdoc {
			return true
		}
		if rd.Word == nil || rd.Hdoc == nil {
			return true
		}

		h := shell.Heredoc{
			Tabbed: rd.Op == syntax.DashHdoc,
			Quoted: isQuotedDelimiter(rd.Word),
			Delim:  wordLiteral(rd.Word),
		}

		// 본문 텍스트를 원본 소스 오프셋으로 추출
		start := rd.Hdoc.Pos().Offset()
		end := rd.Hdoc.End().Offset()
		if int(end) <= len(src) {
			h.Body = strings.TrimRight(src[start:end], "\n")
		}

		result = append(result, h)
		return true
	})

	return result
}

// isQuotedDelimiter 는 heredoc 구분자 Word 가 따옴표로 감싸졌는지 반환한다.
func isQuotedDelimiter(w *syntax.Word) bool {
	if len(w.Parts) == 0 {
		return false
	}
	switch w.Parts[0].(type) {
	case *syntax.SglQuoted, *syntax.DblQuoted:
		return true
	}
	return false
}

// wordLiteral 은 Word 에서 리터럴 텍스트만 추출한다.
func wordLiteral(w *syntax.Word) string {
	var sb strings.Builder
	for _, p := range w.Parts {
		switch v := p.(type) {
		case *syntax.Lit:
			sb.WriteString(v.Value)
		case *syntax.SglQuoted:
			sb.WriteString(v.Value)
		case *syntax.DblQuoted:
			for _, dp := range v.Parts {
				if lit, ok := dp.(*syntax.Lit); ok {
					sb.WriteString(lit.Value)
				}
			}
		}
	}
	return sb.String()
}
