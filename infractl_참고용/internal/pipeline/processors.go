// Package pipeline
// File: processors.go
// Description: 명령 출력 라인 전처리 함수 모음
// Responsibility: ANSI 제거, 공백 정규화, CLI 노이즈 제거 등 순수 변환 함수 제공

package pipeline

import (
	"regexp"
	"strings"
)

// ProcessorFunc는 단일 출력 라인을 변환하는 함수 타입이다.
type ProcessorFunc func(line string) string

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// StripANSI는 ANSI 이스케이프 시퀀스를 제거한다.
func StripANSI(line string) string {
	return ansiEscape.ReplaceAllString(line, "")
}

// CompressWhitespace는 연속된 공백/탭을 단일 공백으로 압축하고 양 끝 공백을 제거한다.
func CompressWhitespace(line string) string {
	fields := strings.Fields(line)
	return strings.Join(fields, " ")
}

// StripSQLPlusNoise는 sqlplus CLI 특유의 노이즈 라인을 빈 문자열로 변환한다.
// 빈 문자열을 반환하면 파이프라인에서 해당 라인을 건너뛴다.
func StripSQLPlusNoise(line string) string {
	trimmed := strings.TrimSpace(line)

	// SQL*Plus 배너
	if strings.HasPrefix(trimmed, "SQL*Plus: Release") {
		return ""
	}
	if strings.HasPrefix(trimmed, "Copyright (c) 1982") {
		return ""
	}
	if strings.HasPrefix(trimmed, "Connected to:") {
		return ""
	}
	if strings.HasPrefix(trimmed, "Oracle Database") && strings.Contains(trimmed, "Release") {
		return ""
	}

	// SQL> 프롬프트
	if trimmed == "SQL>" || strings.HasPrefix(trimmed, "SQL> ") {
		return ""
	}

	// 구분선 (-----)
	if isOnlyDashes(trimmed) {
		return ""
	}

	// 빈 라인 유지 (데이터 구조 보존)
	return line
}

// StripMySQLNoise는 mysql CLI 특유의 노이즈 라인을 빈 문자열로 변환한다.
func StripMySQLNoise(line string) string {
	trimmed := strings.TrimSpace(line)

	// mysql 배너
	if strings.HasPrefix(trimmed, "mysql: [Warning]") {
		return ""
	}
	if strings.HasPrefix(trimmed, "Welcome to the MySQL monitor") {
		return ""
	}
	if strings.HasPrefix(trimmed, "Your MySQL connection id") {
		return ""
	}
	if strings.HasPrefix(trimmed, "Server version:") {
		return ""
	}

	// mysql> 프롬프트
	if trimmed == "mysql>" || strings.HasPrefix(trimmed, "mysql> ") {
		return ""
	}

	// +---+ 형태의 구분선
	if isTableBorder(trimmed) {
		return ""
	}

	return line
}

// StripPSQLNoise는 psql CLI 특유의 노이즈 라인을 빈 문자열로 변환한다.
func StripPSQLNoise(line string) string {
	trimmed := strings.TrimSpace(line)

	// psql 배너
	if strings.HasPrefix(trimmed, "psql (") {
		return ""
	}
	if strings.HasPrefix(trimmed, "Type \"help\"") {
		return ""
	}
	if strings.HasPrefix(trimmed, "SSL connection") {
		return ""
	}

	// psql 프롬프트 (database=#, database=>)
	if isPSQLPrompt(trimmed) {
		return ""
	}

	// 타이밍 라인
	if strings.HasPrefix(trimmed, "Time: ") {
		return ""
	}

	return line
}

// --- 내부 헬퍼 ---

func isOnlyDashes(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, r := range s {
		if r != '-' && r != ' ' {
			return false
		}
	}
	return true
}

func isTableBorder(s string) bool {
	if len(s) < 3 {
		return false
	}
	return strings.HasPrefix(s, "+") && strings.HasSuffix(s, "+") &&
		strings.ContainsAny(s, "-")
}

func isPSQLPrompt(s string) bool {
	// "dbname=# ", "dbname=> " 형태
	if idx := strings.Index(s, "=# "); idx > 0 {
		return true
	}
	if idx := strings.Index(s, "=> "); idx > 0 {
		return true
	}
	return false
}
