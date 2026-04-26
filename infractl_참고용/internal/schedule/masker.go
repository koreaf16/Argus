// Package schedule
// File: masker.go
// Description: 스케줄 로그 크리덴셜 마스킹
// Responsibility: 로그 기록 전 password/token/api_key 등 민감 값을 *** 로 치환

package schedule

import "regexp"

// maskPatterns는 크리덴셜 패턴 목록이다.
// 예: password=secret → password=***
var maskPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(password\s*[=:]\s*)\S+`),
	regexp.MustCompile(`(?i)(passwd\s*[=:]\s*)\S+`),
	regexp.MustCompile(`(?i)(token\s*[=:]\s*)\S+`),
	regexp.MustCompile(`(?i)(api[_-]?key\s*[=:]\s*)\S+`),
	regexp.MustCompile(`(?i)(secret\s*[=:]\s*)\S+`),
	regexp.MustCompile(`(?i)(auth\s*[=:]\s*)\S+`),
}

// MaskCredentials는 s 에서 민감 값을 *** 로 치환하여 반환한다.
func MaskCredentials(s string) string {
	for _, re := range maskPatterns {
		s = re.ReplaceAllString(s, "${1}***")
	}
	return s
}
