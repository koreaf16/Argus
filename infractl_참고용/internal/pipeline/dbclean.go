// Package pipeline
// File: dbclean.go
// Description: DB CLI 출력 노이즈 제거 함수
// Responsibility: sqlplus/mysql/psql CLI 래퍼가 생성하는 배너·프롬프트를 제거하여 순수 데이터 반환

package pipeline

import "strings"

// CleanSQLPlusOutput은 sqlplus CLI 출력에서 배너, 프롬프트, 구분선을 제거한다.
// Oracle 쿼리 결과의 실제 데이터 라인만 반환한다.
func CleanSQLPlusOutput(raw string) string {
	lines := strings.Split(raw, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		result := StripSQLPlusNoise(line)
		if result != "" {
			cleaned = append(cleaned, result)
		}
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

// CleanMySQLOutput은 mysql CLI 출력에서 배너, 프롬프트, 구분선을 제거한다.
// MySQL 쿼리 결과의 실제 데이터 라인만 반환한다.
func CleanMySQLOutput(raw string) string {
	lines := strings.Split(raw, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		result := StripMySQLNoise(line)
		if result != "" {
			cleaned = append(cleaned, result)
		}
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

// CleanPSQLOutput은 psql CLI 출력에서 배너, 프롬프트, 타이밍 라인을 제거한다.
// PostgreSQL 쿼리 결과의 실제 데이터 라인만 반환한다.
func CleanPSQLOutput(raw string) string {
	lines := strings.Split(raw, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		result := StripPSQLNoise(line)
		if result != "" {
			cleaned = append(cleaned, result)
		}
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}
