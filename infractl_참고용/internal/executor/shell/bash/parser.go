// Package bash
// File: parser.go
// Description: mvdan.cc/sh/v3 기반 bash AST 파싱 진입점
// Responsibility: Parse — 원시 bash 소스를 *syntax.File 로 변환, Walk — AST 순회 위임

// Ported from: claude_cli/src/utils/bash/bashParser.ts
// Go에서는 tree-sitter 대신 mvdan.cc/sh/v3 를 직접 사용한다.

package bash

import (
	"context"
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Parse 는 bash 소스 문자열을 AST 로 파싱한다.
// 파싱 실패 시 에러를 반환하며, 호출자는 FAIL-CLOSED 원칙에 따라
// Analysis.RiskLevel = LevelUserApproval 로 처리해야 한다.
func Parse(_ context.Context, src string) (*syntax.File, error) {
	r := strings.NewReader(src)
	f, err := syntax.NewParser(
		syntax.KeepComments(true),
		syntax.Variant(syntax.LangBash),
	).Parse(r, "")
	if err != nil {
		return nil, fmt.Errorf("bash parse: %w", err)
	}
	return f, nil
}

// Walk 는 AST 를 순회하며 visitor 를 호출한다.
// visitor 가 false 를 반환하면 해당 노드의 하위를 건너뛴다.
func Walk(f *syntax.File, visitor func(syntax.Node) bool) {
	syntax.Walk(f, visitor)
}
