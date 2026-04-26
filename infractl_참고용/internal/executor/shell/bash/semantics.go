// Package bash
// File: semantics.go
// Description: bash AST 기반 FAIL-CLOSED 위험도 분석
// Responsibility: CheckSemantics — AST 순회, danger score, readonly 판정

// Ported from: claude_cli/src/utils/bash/ast.ts:parseForSecurity / checkSemantics

package bash

import (
	"context"
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/yourorg/infractl/internal/executor/shell"
	"github.com/yourorg/infractl/internal/executor/shell/bash/danger"
	"github.com/yourorg/infractl/internal/executor/shell/bash/readonly"
)

// pre-check 정규식: 명령어 문자열 수준에서 분석 불가 패턴을 조기 탐지한다.
// Ported from: claude_cli/src/utils/bash/ast.ts (CONTROL_CHAR_RE 등)
var (
	controlCharRE         = regexp.MustCompile(`[\x00-\x08\x0e-\x1f\x7f-\x9f]`)
	unicodeWhitespaceRE   = regexp.MustCompile("[\u00a0\u2000-\u200b\u2028\u2029\u3000]")
	backslashWhitespaceRE = regexp.MustCompile(`\\\s`)
	zshTildeBracketRE     = regexp.MustCompile(`~\[`)
	zshEqualsExpansionRE  = regexp.MustCompile(`=\(`)
)

// CheckSemantics 는 src 문자열과 파싱된 AST f 를 분석해 Analysis 를 반환한다.
// 파싱 실패나 분석 불가 패턴 → LevelUserApproval (FAIL-CLOSED).
// Ported from: claude_cli/src/utils/bash/ast.ts:parseForSecurity
func CheckSemantics(_ context.Context, src string, f *syntax.File) *shell.Analysis {
	a := &shell.Analysis{
		RawCommand: src,
		ShellName:  "bash",
		IsReadOnly: true,
		RiskLevel:  shell.LevelLow,
	}

	// Extract heredocs before pre-checks (heredoc body may contain control chars legitimately)
	a.Heredocs = Extract(f, src)

	// Pre-checks on the raw string
	if reason := preCheck(src); reason != "" {
		a.IsReadOnly = false
		a.RiskLevel = shell.LevelUserApproval
		a.Findings = append(a.Findings, shell.Finding{
			Node:   "PreCheck",
			Reason: reason,
			Level:  shell.LevelUserApproval,
			Ported: "ast.ts:CONTROL_CHAR_RE/UNICODE_WHITESPACE_RE",
		})
		a.DangerScore = danger.ScoreFromLevel(shell.LevelUserApproval)
		return a
	}

	// Walk AST collecting all command calls, pipes, redirects, subshells
	w := &walker{src: src, a: a}
	syntax.Walk(f, w.visit)

	// Final score from accumulated risk level
	a.DangerScore = danger.ScoreFromLevel(a.RiskLevel)
	return a
}

// preCheck 는 원본 문자열에서 분석 불가 패턴을 검사한다.
// 문제가 없으면 "" 를 반환하고, 문제가 있으면 이유 문자열을 반환한다.
// Ported from: claude_cli/src/utils/bash/ast.ts (precheck section)
func preCheck(src string) string {
	if controlCharRE.MatchString(src) {
		return "제어 문자가 포함된 명령입니다"
	}
	if unicodeWhitespaceRE.MatchString(src) {
		return "비표준 유니코드 공백이 포함된 명령입니다"
	}
	if backslashWhitespaceRE.MatchString(src) {
		return "백슬래시+공백 패턴이 포함된 명령입니다"
	}
	if zshTildeBracketRE.MatchString(src) {
		return "ZSH ~[ 확장 패턴이 포함되어 있습니다"
	}
	if zshEqualsExpansionRE.MatchString(src) {
		return "ZSH =( 확장 패턴이 포함되어 있습니다"
	}
	return ""
}

// walker 는 AST 방문자 상태를 담는다.
type walker struct {
	src      string
	a        *shell.Analysis
	inCmdSub bool // CmdSubst 내부 중복 방지용
}

// visit 는 syntax.Walk 의 visitor 함수다.
func (w *walker) visit(n syntax.Node) bool {
	if n == nil {
		return false
	}
	switch x := n.(type) {
	case *syntax.CallExpr:
		w.handleCallExpr(x)
		return true
	case *syntax.BinaryCmd:
		if x.Op == syntax.Pipe {
			w.handlePipe(x)
			return false // 파이프 자식은 handlePipe 에서 별도 처리
		}
	case *syntax.CmdSubst:
		w.handleCmdSubst(x)
		return false // 재귀 방지: 내부 노드는 내부 walk 에서 처리
	case *syntax.ProcSubst:
		w.handleProcSubst(x)
		return false
	case *syntax.Redirect:
		w.handleRedirect(x)
	case *syntax.FuncDecl:
		w.handleFuncDecl(x)
	}
	return true
}

// handleCallExpr 는 단일 명령 호출을 분석한다.
func (w *walker) handleCallExpr(x *syntax.CallExpr) {
	argv := extractArgv(x)
	if len(argv) == 0 {
		return
	}
	// 절대경로 명령(/usr/bin/ls → ls) 정규화
	if idx := strings.LastIndex(argv[0], "/"); idx >= 0 {
		argv[0] = argv[0][idx+1:]
	}
	name := argv[0]
	args := argv[1:]

	// Readonly whitelist 우선 확인
	if readonly.IsReadOnly(argv) {
		seg := shell.PipeSegment{CallExpr: strings.Join(argv, " "), IsReadOnly: true, Level: shell.LevelLow}
		w.a.PipeCommands = append(w.a.PipeCommands, seg)
		w.a.MatchedSpecs = append(w.a.MatchedSpecs, name+":read-only")
		w.a.RiskLevel = danger.MaxLevel(w.a.RiskLevel, shell.LevelLow)
		return
	}

	// 화이트리스트 미포함 → 쓰기 가능 명령
	w.a.IsReadOnly = false

	lvl, reason := danger.CheckCallExpr(name, args)
	seg := shell.PipeSegment{CallExpr: strings.Join(argv, " "), IsReadOnly: false, Level: lvl}
	w.a.PipeCommands = append(w.a.PipeCommands, seg)
	w.a.MatchedSpecs = append(w.a.MatchedSpecs, name)

	if lvl > shell.LevelLow {
		w.a.Findings = append(w.a.Findings, shell.Finding{
			Node:   "CallExpr",
			Reason: reason,
			Level:  lvl,
			Ported: "ast.ts:checkSemantics",
		})
	}
	w.a.RiskLevel = danger.MaxLevel(w.a.RiskLevel, lvl)
}

// handlePipe 는 파이프(|) 양쪽을 개별 분석하고, curl|sh 패턴을 탐지한다.
func (w *walker) handlePipe(x *syntax.BinaryCmd) {
	leftName := callExprNameFromStmt(x.X)
	rightName := callExprNameFromStmt(x.Y)

	// curl|sh 등 원격 코드 실행 패턴
	if lvl, reason := danger.CheckPipe(leftName, rightName); lvl > shell.LevelLow {
		w.a.IsReadOnly = false
		w.a.Findings = append(w.a.Findings, shell.Finding{
			Node:   "Pipe",
			Reason: reason,
			Level:  lvl,
			Ported: "ast.ts:checkSemantics (pipe pattern)",
		})
		w.a.RiskLevel = danger.MaxLevel(w.a.RiskLevel, lvl)
	}

	// 파이프 양측을 별도로 walk
	syntax.Walk(x.X, w.visit)
	syntax.Walk(x.Y, w.visit)
}

// handleCmdSubst 는 $(...) 명령 치환을 High 위험으로 기록한다.
// 내부 Stmts 를 직접 walk 하여 CmdSubst 노드 자체의 재방문을 방지한다.
func (w *walker) handleCmdSubst(x *syntax.CmdSubst) {
	start, end := int(x.Pos().Offset()), int(x.End().Offset())
	snippet := ""
	if start < len(w.src) && end <= len(w.src) {
		snippet = w.src[start:end]
	}
	w.a.Subshells = append(w.a.Subshells, snippet)
	w.a.IsReadOnly = false

	elevated := danger.MaxLevel(w.a.RiskLevel, shell.LevelHigh)
	if elevated > w.a.RiskLevel {
		w.a.RiskLevel = elevated
		w.a.Findings = append(w.a.Findings, shell.Finding{
			Node:   "CmdSubst",
			Reason: "명령 치환 $(...) — 동적 코드 실행",
			Level:  shell.LevelHigh,
			Ported: "ast.ts:checkSemantics (CmdSubst)",
		})
	}

	// 내부 Stmt 목록만 walk (CmdSubst 노드 자체를 다시 방문하지 않음)
	inner := &walker{src: w.src, a: w.a, inCmdSub: true}
	for _, stmt := range x.Stmts {
		syntax.Walk(stmt, inner.visit)
	}
}

// handleProcSubst 는 <(...) / >(...) 프로세스 치환을 High 로 기록한다.
func (w *walker) handleProcSubst(x *syntax.ProcSubst) {
	start, end := int(x.Pos().Offset()), int(x.End().Offset())
	snippet := ""
	if start < len(w.src) && end <= len(w.src) {
		snippet = w.src[start:end]
	}
	w.a.Subshells = append(w.a.Subshells, snippet)
	w.a.IsReadOnly = false

	elevated := danger.MaxLevel(w.a.RiskLevel, shell.LevelHigh)
	if elevated > w.a.RiskLevel {
		w.a.RiskLevel = elevated
		w.a.Findings = append(w.a.Findings, shell.Finding{
			Node:   "ProcSubst",
			Reason: "프로세스 치환 <(...) — 동적 코드 실행",
			Level:  shell.LevelHigh,
			Ported: "ast.ts:checkSemantics (ProcSubst)",
		})
	}
	inner := &walker{src: w.src, a: w.a, inCmdSub: true}
	for _, stmt := range x.Stmts {
		syntax.Walk(stmt, inner.visit)
	}
}

// safeRedirectTargets 는 쓰기해도 위험하지 않은 특수 경로 목록이다.
var safeRedirectTargets = map[string]bool{
	"/dev/null":   true,
	"/dev/stdin":  true,
	"/dev/stdout": true,
	"/dev/stderr": true,
}

// handleRedirect 는 쓰기 리다이렉트를 탐지한다.
// /dev/null 등 안전한 타겟은 IsReadOnly 에 영향 없음.
// 그 외 쓰기 리다이렉트는 IsReadOnly=false 로 표시하고 위험도를 격상한다.
// Ported from: claude_cli/src/utils/bash/ast.ts:checkSemantics (redirect)
func (w *walker) handleRedirect(x *syntax.Redirect) {
	isWrite := x.Op == syntax.RdrOut || x.Op == syntax.AppOut ||
		x.Op == syntax.RdrAll || x.Op == syntax.AppAll
	if !isWrite || x.Word == nil {
		return
	}

	target := wordLiteral(x.Word)
	if safeRedirectTargets[target] {
		return // /dev/null 등 안전한 타겟은 읽기 전용 판정에 영향 없음
	}

	// 안전하지 않은 쓰기 리다이렉트 → 읽기 전용 아님
	w.a.IsReadOnly = false

	if target == "" {
		return
	}

	var lvl shell.Level
	var reason string
	switch {
	case strings.HasPrefix(target, "/dev/"):
		lvl = shell.LevelCritical
		reason = "디바이스 파일에 직접 씁니다: " + target
	case strings.HasPrefix(target, "/etc/") || strings.HasPrefix(target, "/sys/") || strings.HasPrefix(target, "/proc/"):
		lvl = shell.LevelHigh
		reason = "시스템 경로에 씁니다: " + target
	default:
		lvl = shell.LevelMedium
		reason = "파일에 씁니다: " + target
	}

	w.a.Findings = append(w.a.Findings, shell.Finding{
		Node:   "Redirect",
		Reason: reason,
		Level:  lvl,
		Ported: "ast.ts:checkSemantics (redirect)",
	})
	w.a.RiskLevel = danger.MaxLevel(w.a.RiskLevel, lvl)
}

// handleFuncDecl 는 함수 선언을 검사한다.
// 이름이 `:` 인 함수는 fork bomb 후보로 Critical 처리한다.
func (w *walker) handleFuncDecl(x *syntax.FuncDecl) {
	if x.Name != nil && x.Name.Value == ":" {
		w.a.IsReadOnly = false
		w.a.Findings = append(w.a.Findings, shell.Finding{
			Node:   "FuncDecl",
			Reason: "fork bomb 패턴 (:(){ :|:& };:)",
			Level:  shell.LevelCritical,
			Ported: "ast.ts:checkSemantics (fork bomb)",
		})
		w.a.RiskLevel = danger.MaxLevel(w.a.RiskLevel, shell.LevelCritical)
	}
}

// extractArgv 는 *syntax.CallExpr 에서 argv 문자열 목록을 추출한다.
// 변수 확장이 포함된 경우 리터럴 부분만 추출한다.
func extractArgv(x *syntax.CallExpr) []string {
	result := make([]string, 0, len(x.Args))
	for _, w := range x.Args {
		result = append(result, wordLiteral(w))
	}
	return result
}

// callExprNameFromStmt 는 *syntax.Stmt 의 Cmd 가 단순 CallExpr 일 때 명령 이름을 반환한다.
// BinaryCmd.X/Y 는 *syntax.Stmt 이므로 이 함수를 사용한다.
func callExprNameFromStmt(stmt *syntax.Stmt) string {
	if stmt == nil {
		return ""
	}
	c, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || len(c.Args) == 0 {
		return ""
	}
	return wordLiteral(c.Args[0])
}
