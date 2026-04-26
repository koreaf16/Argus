// Package bash — 파싱된 명령 인터페이스와 구현체.
//
// 파일 역할: IParsedCommand 인터페이스와 정규식 폴백 구현체를 제공한다.
//
//	파이프 세그먼트 분리, 출력 리다이렉션 제거, tree-sitter 분석 결과 접근을 지원한다.
//
// 포함 모듈:
//   - OutputRedirection: 리다이렉션 연산자와 대상
//   - IParsedCommand: 파싱 명령 공통 인터페이스
//   - RegexParsedCommand: 정규식 기반 폴백 구현체 (tree-sitter 불가 시)
//   - ParsedCommand: ParsedCommand.Parse(command) 진입점
//
// 호출/사용 방식:
//   - internal/utils/permissions/bash_classifier.go 가 파이프 세그먼트 분석에 사용
//   - internal/tools/bash/bash_tool.go 가 리다이렉션 제거에 사용
//
// 연결:
//   - 원본: src/utils/bash/ParsedCommand.ts
package bash

import "strings"

// OutputRedirection 은 출력 리다이렉션 연산자와 대상 파일을 나타낸다.
type OutputRedirection struct {
	Target   string
	Operator string // ">" 또는 ">>"
}

// IParsedCommand 는 파싱된 명령의 공통 인터페이스다.
type IParsedCommand interface {
	// OriginalCommand 는 원본 명령 문자열을 반환한다.
	OriginalCommand() string
	// String 은 명령 문자열 표현을 반환한다.
	String() string
	// GetPipeSegments 는 파이프로 분리된 세그먼트 슬라이스를 반환한다.
	GetPipeSegments() []string
	// WithoutOutputRedirections 는 출력 리다이렉션이 제거된 명령 문자열을 반환한다.
	WithoutOutputRedirections() string
	// GetOutputRedirections 는 출력 리다이렉션 목록을 반환한다.
	GetOutputRedirections() []OutputRedirection
	// GetTreeSitterAnalysis 는 tree-sitter 분석 결과를 반환한다 (없으면 nil).
	GetTreeSitterAnalysis() *TreeSitterAnalysisResult
}

// RegexParsedCommand 는 정규식 기반 폴백 파싱 구현체다.
// tree-sitter 가 없는 Phase 1 에서 사용한다.
type RegexParsedCommand struct {
	original string
}

// NewRegexParsedCommand 는 RegexParsedCommand 를 생성한다.
func NewRegexParsedCommand(command string) *RegexParsedCommand {
	return &RegexParsedCommand{original: command}
}

func (r *RegexParsedCommand) OriginalCommand() string { return r.original }
func (r *RegexParsedCommand) String() string          { return r.original }

// GetPipeSegments 는 '|' 연산자 기준으로 명령을 세그먼트로 분리한다.
// 단순 분리이므로 인용 부호 내의 '|' 는 구분하지 않는다.
func (r *RegexParsedCommand) GetPipeSegments() []string {
	parts := strings.Split(r.original, "|")
	segments := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			segments = append(segments, trimmed)
		}
	}
	if len(segments) == 0 {
		return []string{r.original}
	}
	return segments
}

// WithoutOutputRedirections 는 '>' 와 '>>' 리다이렉션을 제거한 명령을 반환한다.
func (r *RegexParsedCommand) WithoutOutputRedirections() string {
	if !strings.Contains(r.original, ">") {
		return r.original
	}
	_, redirections := extractOutputRedirections(r.original)
	if len(redirections) == 0 {
		return r.original
	}
	cmd, _ := extractOutputRedirections(r.original)
	return strings.TrimSpace(cmd)
}

// GetOutputRedirections 는 명령의 출력 리다이렉션 목록을 반환한다.
func (r *RegexParsedCommand) GetOutputRedirections() []OutputRedirection {
	_, redirections := extractOutputRedirections(r.original)
	return redirections
}

// GetTreeSitterAnalysis 는 항상 nil 을 반환한다 (폴백 구현).
func (r *RegexParsedCommand) GetTreeSitterAnalysis() *TreeSitterAnalysisResult {
	return nil
}

// extractOutputRedirections 는 command 에서 '>' 와 '>>' 리다이렉션을 추출한다.
// 반환값: (리다이렉션 제거된 명령, 리다이렉션 목록).
func extractOutputRedirections(command string) (string, []OutputRedirection) {
	var redirections []OutputRedirection
	result := command

	// '>>' 를 먼저 처리 ('>>' 가 '>' 보다 우선)
	for {
		idx := strings.Index(result, ">>")
		if idx < 0 {
			break
		}
		rest := strings.TrimSpace(result[idx+2:])
		parts := strings.Fields(rest)
		if len(parts) == 0 {
			break
		}
		target := parts[0]
		redirections = append(redirections, OutputRedirection{Target: target, Operator: ">>"})
		result = result[:idx] + " " + strings.Join(parts[1:], " ")
	}

	for {
		idx := strings.Index(result, ">")
		if idx < 0 {
			break
		}
		// '>&' 는 fd 리다이렉션이므로 건너뜀
		if idx+1 < len(result) && result[idx+1] == '&' {
			break
		}
		rest := strings.TrimSpace(result[idx+1:])
		parts := strings.Fields(rest)
		if len(parts) == 0 {
			break
		}
		target := parts[0]
		redirections = append(redirections, OutputRedirection{Target: target, Operator: ">"})
		result = result[:idx] + " " + strings.Join(parts[1:], " ")
	}

	return strings.TrimSpace(result), redirections
}

// ParsedCommandFactory 는 명령을 파싱해 IParsedCommand 를 반환하는 팩토리다.
var ParsedCommandFactory = struct {
	Parse func(command string) IParsedCommand
}{
	Parse: func(command string) IParsedCommand {
		return NewRegexParsedCommand(command)
	},
}
