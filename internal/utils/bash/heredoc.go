// Package bash — 히어독(heredoc) 감지 및 처리.
//
// 파일 역할: bash 히어독 구문(<<EOF, <<'EOF', <<"EOF", <<-EOF 등)을 감지하고
//
//	파싱 전 플레이스홀더로 대체했다가 복원하는 기능을 제공한다.
//
// 포함 모듈:
//   - ContainsHeredoc: 명령에 히어독 구문이 포함되어 있는지 확인
//   - ExtractHeredocContent: 히어독 내용 슬라이스 추출
//   - ExtractHeredocs: 히어독을 플레이스홀더로 대체하고 맵 반환
//   - RestoreHeredocs: 플레이스홀더를 원래 히어독으로 복원
//
// 호출/사용 방식:
//   - internal/utils/bash/commands.go 의 SplitCommandWithOperators 가 파싱 전에 호출
//   - internal/utils/bash/shell_quoting.go 가 히어독 감지에 사용
//
// 연결:
//   - 원본: src/utils/bash/heredoc.ts
package bash

import (
	"fmt"
	"regexp"
	"strings"
)

// heredocStartRe 는 히어독 시작 패턴을 매칭한다.
// <<[-]?['"]?WORD['"]? 형식을 지원한다.
// Go regexp 는 \1 백레퍼런스를 지원하지 않으므로 단순화한다.
var heredocStartRe = regexp.MustCompile(`<<-?\s*(?:['"](\w+)['"]|(\w+)|\\(\w+))`)

// bitShiftRe 는 비트시프트 연산자를 감지해 히어독에서 제외한다.
var bitShiftRe = regexp.MustCompile(`\d\s*<<\s*\d|\[\[\s*\d+\s*<<\s*\d+\s*\]\]|\$\(\(.*<<.*\)\)`)

// ContainsHeredoc 는 command 에 히어독 구문이 포함되어 있는지 반환한다.
func ContainsHeredoc(command string) bool {
	if bitShiftRe.MatchString(command) {
		return false
	}
	return heredocStartRe.MatchString(command)
}

// ExtractHeredocContent 는 command 에서 히어독 내용 슬라이스를 반환한다.
// 히어독이 없으면 nil 을 반환한다.
func ExtractHeredocContent(command string) []string {
	if !ContainsHeredoc(command) {
		return nil
	}

	var contents []string
	lines := strings.Split(command, "\n")

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		matches := heredocStartRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		// 구분자 추출 (인용 부호 제거)
		delimiter := matches[1]
		if delimiter == "" {
			delimiter = matches[2]
		}
		if delimiter == "" {
			delimiter = matches[3]
		}
		if delimiter == "" {
			continue
		}

		// 히어독 내용 수집
		var content []string
		i++
		for i < len(lines) {
			stripped := strings.TrimLeft(lines[i], "\t")
			if stripped == delimiter || lines[i] == delimiter {
				break
			}
			content = append(content, lines[i])
			i++
		}
		contents = append(contents, strings.Join(content, "\n"))
	}

	return contents
}

// HeredocExtractionResult 는 히어독 추출 결과다.
type HeredocExtractionResult struct {
	ProcessedCommand string
	Heredocs         map[string]string // 플레이스홀더 → 원본 히어독
}

// ExtractHeredocs 는 command 에서 히어독을 플레이스홀더로 대체하고 맵을 반환한다.
func ExtractHeredocs(command string) HeredocExtractionResult {
	if !ContainsHeredoc(command) {
		return HeredocExtractionResult{ProcessedCommand: command, Heredocs: map[string]string{}}
	}

	heredocs := make(map[string]string)
	processed := command
	counter := 0

	for ContainsHeredoc(processed) {
		loc := heredocStartRe.FindStringIndex(processed)
		if loc == nil {
			break
		}

		// 히어독 전체 범위 찾기
		match := heredocStartRe.FindStringSubmatch(processed[loc[0]:])
		delimiter := match[1]
		if delimiter == "" {
			delimiter = match[2]
		}
		if delimiter == "" {
			delimiter = match[3]
		}
		if delimiter == "" {
			break
		}

		// 첫 번째 줄 끝 찾기
		nlIdx := strings.Index(processed[loc[1]:], "\n")
		if nlIdx < 0 {
			break
		}
		bodyStart := loc[1] + nlIdx + 1

		// 구분자로 끝나는 줄 찾기
		endMarker := "\n" + delimiter
		endIdx := strings.Index(processed[bodyStart:], endMarker)
		if endIdx < 0 {
			break
		}
		heredocEnd := bodyStart + endIdx + len(endMarker)

		placeholder := fmt.Sprintf("__HEREDOC_%d__", counter)
		counter++
		heredocs[placeholder] = processed[loc[0]:heredocEnd]
		processed = processed[:loc[0]] + placeholder + processed[heredocEnd:]
	}

	return HeredocExtractionResult{ProcessedCommand: processed, Heredocs: heredocs}
}

// RestoreHeredocs 는 processed 에서 플레이스홀더를 원래 히어독으로 복원한다.
func RestoreHeredocs(processed string, heredocs map[string]string) string {
	result := processed
	for placeholder, original := range heredocs {
		result = strings.ReplaceAll(result, placeholder, original)
	}
	return result
}
