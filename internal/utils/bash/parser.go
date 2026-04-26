// Package bash — 셸 명령 파서.
//
// 파일 역할: bash 명령 문자열을 토큰으로 분리하는 경량 파서를 제공한다.
//
//	Phase 1 에서는 tree-sitter Go 바인딩 없이 순수 Go 로 구현한다.
//	bashParser.ts 와 parser.ts 의 핵심 API 를 포팅한다.
//
// 포함 모듈:
//   - ParsedResult: 파싱된 명령 루트 노드와 명령 노드 포인터
//   - ParseCommand: 명령 문자열을 ParsedResult 로 파싱 (nil 반환 = 파싱 불가)
//   - ExtractCommandArguments: Node 에서 인수 슬라이스 추출
//   - tokenizeSimple: 인용 부호를 존중하는 단순 토크나이저 (내부 공유)
//
// 호출/사용 방식:
//   - internal/utils/bash/prefix.go 가 ParseCommand 를 호출해 명령 접두사를 추출
//   - internal/utils/bash/ast.go 가 tokenizeSimple 을 사용
//
// 연결:
//   - 원본: src/utils/bash/bashParser.ts + src/utils/bash/parser.ts
package bash

import "strings"

// ShellKeywords 는 bash 예약어 집합이다.
var ShellKeywords = map[string]bool{
	"if": true, "then": true, "elif": true, "else": true, "fi": true,
	"while": true, "until": true, "for": true, "in": true, "do": true,
	"done": true, "case": true, "esac": true, "select": true,
	"function": true, "time": true, "coproc": true,
	"!": true, "{": true, "}": true, "[[": true, "]]": true,
}

// ParsedResult 는 파싱 결과를 담는다.
type ParsedResult struct {
	RootNode    *Node
	CommandNode *Node    // 첫 번째 simple_command 노드
	EnvVars     []string // "KEY=VALUE" 형식의 환경변수
}

// ParseCommand 는 command 를 파싱해 ParsedResult 를 반환한다.
// 파싱에 실패하거나 명령이 비어있으면 nil 을 반환한다.
// Phase 1: 단순 토크나이저 기반 구현.
func ParseCommand(command string) *ParsedResult {
	if strings.TrimSpace(command) == "" {
		return nil
	}

	tokens := tokenizeSimple(command)
	if len(tokens) == 0 {
		return nil
	}

	// 환경변수 접두사 분리
	var envVars []string
	cmdTokens := tokens
	for i, tok := range tokens {
		if isEnvVarAssignment(tok) {
			envVars = append(envVars, tok)
		} else {
			cmdTokens = tokens[i:]
			break
		}
	}

	// AST 노드 구성
	children := make([]*Node, 0, len(cmdTokens))
	for _, tok := range cmdTokens {
		children = append(children, &Node{Type: "word", Text: tok})
	}

	cmdNode := &Node{
		Type:     "simple_command",
		Text:     command,
		Children: children,
	}

	root := &Node{
		Type:     "program",
		Text:     command,
		Children: []*Node{cmdNode},
	}

	return &ParsedResult{
		RootNode:    root,
		CommandNode: cmdNode,
		EnvVars:     envVars,
	}
}

// ExtractCommandArguments 는 simple_command 노드에서 argv 슬라이스를 추출한다.
func ExtractCommandArguments(node *Node) []string {
	if node == nil {
		return nil
	}
	args := make([]string, 0, len(node.Children))
	for _, child := range node.Children {
		if child.Type == "word" || child.Type == "number" {
			args = append(args, child.Text)
		}
	}
	return args
}

// tokenizeSimple 은 command 를 인용 부호를 존중해 토큰 슬라이스로 분리한다.
// 단일/이중 인용 부호를 처리하며, 셸 연산자는 독립 토큰으로 취급하지 않는다.
func tokenizeSimple(command string) []string {
	var tokens []string
	var current strings.Builder
	inSingle := false
	inDouble := false

	for i := 0; i < len(command); i++ {
		ch := command[i]
		switch {
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
		case ch == '"' && !inSingle:
			inDouble = !inDouble
		case ch == '\\' && !inSingle && i+1 < len(command):
			i++
			current.WriteByte(command[i])
		case (ch == ' ' || ch == '\t' || ch == '\n') && !inSingle && !inDouble:
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// isEnvVarAssignment 는 token 이 VAR=VALUE 형식인지 검사한다.
func isEnvVarAssignment(token string) bool {
	if len(token) == 0 {
		return false
	}
	// 이름은 문자 또는 '_' 로 시작해야 한다
	ch := token[0]
	if !((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || ch == '_') {
		return false
	}
	for i, c := range token {
		if c == '=' && i > 0 {
			return true
		}
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return false
}
