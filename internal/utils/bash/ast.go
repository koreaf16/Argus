// Package bash — 셸 AST 노드 타입 정의.
//
// 파일 역할: tree-sitter-bash 호환 AST 노드 구조체와 보안 분석 결과 타입을 정의한다.
//
//	ast.ts 의 ParseForSecurityResult, SimpleCommand, Redirect 타입을 포팅한다.
//
// 포함 모듈:
//   - Node: AST 노드 (type/text/startIndex/endIndex/children)
//   - Redirect: 리다이렉션 정보 (op, target, fd)
//   - SimpleCommand: 단순 명령 (argv, envVars, redirects, text)
//   - ParseForSecurityResult: 보안 분석 결과 (simple / too-complex / parse-unavailable)
//   - AnalyzeForSecurity: 명령 문자열을 AST 로 분석해 ParseForSecurityResult 반환
//
// 호출/사용 방식:
//   - internal/utils/permissions/bash_classifier.go 가 권한 판단에 사용
//   - internal/tools/bash/bash_tool.go 가 명령 검증에 사용
//
// 연결:
//   - 원본: src/utils/bash/ast.ts
package bash

// Node 는 tree-sitter-bash 호환 AST 노드다.
// startIndex/endIndex 는 UTF-8 바이트 오프셋이다.
type Node struct {
	Type       string
	Text       string
	StartIndex int
	EndIndex   int
	Children   []*Node
}

// Redirect 는 리다이렉션 연산자와 대상을 나타낸다.
type Redirect struct {
	Op     string // ">", ">>", "<", "<<", ">&", ">|", "<&", "&>", "&>>", "<<<"
	Target string
	Fd     int // 명시적 FD (0=stdin, 1=stdout, 2=stderr; -1=미지정)
}

// EnvVarAssignment 는 명령 앞에 붙는 환경변수 할당을 나타낸다.
type EnvVarAssignment struct {
	Name  string
	Value string
}

// SimpleCommand 는 단순 명령 하나를 나타낸다.
// argv[0] 이 명령 이름이고 나머지가 인수다.
type SimpleCommand struct {
	Argv      []string
	EnvVars   []EnvVarAssignment
	Redirects []Redirect
	Text      string // 원본 소스 스팬
}

// ParseForSecurityKind 는 보안 분석 결과 종류다.
type ParseForSecurityKind string

const (
	ParseKindSimple           ParseForSecurityKind = "simple"
	ParseKindTooComplex       ParseForSecurityKind = "too-complex"
	ParseKindParseUnavailable ParseForSecurityKind = "parse-unavailable"
)

// ParseForSecurityResult 는 명령의 보안 분석 결과다.
type ParseForSecurityResult struct {
	Kind     ParseForSecurityKind
	Commands []SimpleCommand // Kind == "simple" 일 때 채워짐
	Reason   string          // Kind == "too-complex" 일 때 채워짐
	NodeType string          // Kind == "too-complex" 일 때 채워짐 (선택적)
}

// AnalyzeForSecurity 는 command 를 파싱해 보안 분석 결과를 반환한다.
// Phase 1: 단순 토크나이징을 사용하는 경량 구현이다.
// tree-sitter Go 바인딩이 필요한 전체 구현은 Phase 2 에서 추가한다.
func AnalyzeForSecurity(command string) ParseForSecurityResult {
	if command == "" {
		return ParseForSecurityResult{Kind: ParseKindSimple, Commands: []SimpleCommand{}}
	}

	// Phase 1: 단순 토크나이저로 argv 추출
	argv := tokenizeSimple(command)
	if len(argv) == 0 {
		return ParseForSecurityResult{Kind: ParseKindSimple, Commands: []SimpleCommand{}}
	}

	// 복잡한 구조 감지 — 있으면 too-complex
	if containsComplexShellSyntax(command) {
		return ParseForSecurityResult{
			Kind:   ParseKindTooComplex,
			Reason: "command contains complex shell syntax",
		}
	}

	cmd := SimpleCommand{
		Argv: argv,
		Text: command,
	}
	return ParseForSecurityResult{Kind: ParseKindSimple, Commands: []SimpleCommand{cmd}}
}

// containsComplexShellSyntax 는 권한 판단을 어렵게 만드는 복잡한 셸 문법을 감지한다.
func containsComplexShellSyntax(command string) bool {
	for _, ch := range command {
		switch ch {
		case '|', ';', '&', '`', '$', '(', ')':
			return true
		}
	}
	return false
}
