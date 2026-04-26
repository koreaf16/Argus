// Package connector
// File: schema_guard.go
// Description: SQL에서 테이블명 추출 및 컬럼 오류 감지를 위한 공통 유틸리티
// Responsibility: DB 종류에 무관하게 SQL을 파싱하여 참조 테이블 목록과 오류 여부를 반환

package connector

import (
	"regexp"
	"strings"
)

// sqlFromClauseRe는 FROM / JOIN 뒤의 테이블/뷰명을 추출한다.
// 서브쿼리의 괄호 직후는 제외한다.
var sqlFromClauseRe = regexp.MustCompile(`(?i)(?:FROM|JOIN)\s+([a-zA-Z_$#][a-zA-Z0-9_$#.]*)`)

// ExtractTableNames는 SQL 문에서 참조하는 테이블/뷰명을 추출하여 대문자로 반환한다.
// 중복은 제거하며, 스키마 접두사(schema.table)에서는 테이블명만 반환한다.
func ExtractTableNames(sql string) []string {
	matches := sqlFromClauseRe.FindAllStringSubmatch(sql, -1)
	seen := make(map[string]struct{}, len(matches))
	var result []string
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		raw := strings.ToUpper(strings.TrimSpace(m[1]))
		// schema.table 형식이면 테이블명만 추출
		if dot := strings.LastIndex(raw, "."); dot >= 0 {
			raw = raw[dot+1:]
		}
		if raw == "" {
			continue
		}
		if _, exists := seen[raw]; exists {
			continue
		}
		seen[raw] = struct{}{}
		result = append(result, raw)
	}
	return result
}

// IsColumnError는 DB 출력에 컬럼 관련 오류가 포함되어 있는지 확인한다.
// Oracle ORA-00904, MySQL Unknown column, PostgreSQL column does not exist를 감지한다.
func IsColumnError(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(output, "ORA-00904") ||
		strings.Contains(lower, "unknown column") ||
		strings.Contains(lower, "column") && strings.Contains(lower, "does not exist") ||
		strings.Contains(lower, "invalid identifier")
}

// IsTableNotFoundError는 DB 출력에 테이블/뷰 미존재 오류가 포함되어 있는지 확인한다.
func IsTableNotFoundError(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(output, "ORA-00942") || // Oracle: table or view does not exist
		strings.Contains(lower, "table") && strings.Contains(lower, "doesn't exist") || // MySQL
		strings.Contains(lower, "relation") && strings.Contains(lower, "does not exist") // PG
}

// HasQueryError는 출력에 어떤 종류의 쿼리 실패 오류가 있는지 확인한다.
func HasQueryError(output string, exitCode int) bool {
	if exitCode != 0 {
		return true
	}
	return IsColumnError(output) || IsTableNotFoundError(output)
}
