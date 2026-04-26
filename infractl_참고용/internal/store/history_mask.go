// Package store
// File: history_mask.go
// Description: 프롬프트 히스토리 저장 전 민감 정보 마스킹 유틸리티
// Responsibility: 비밀번호/API 키 등 민감 패턴을 ***로 치환

package store

import "regexp"

// sensitivePatterns는 마스킹할 민감 정보 패턴 목록이다.
// 각 패턴은 캡처 그룹 1=prefix, 2=민감값 으로 구성된다.
var sensitivePatterns = []*regexp.Regexp{
	// -p password, --password=value, password: value
	regexp.MustCompile(`(?i)((?:-p|--password[= ]|password\s*[:=]\s*))(\S+)`),
	// -P password (대문자, MySQL 등)
	regexp.MustCompile(`(?i)((?:-P\s+))(\S+)`),
	// api_key=value, apikey=value, api-key=value
	regexp.MustCompile(`(?i)((?:api[_-]?key[= ]))([A-Za-z0-9_\-\.]{8,})`),
	// token=value, access_token=value, bearer token
	regexp.MustCompile(`(?i)((?:token[= ]|access_token[= ]|bearer\s+))([A-Za-z0-9_\-\.]{8,})`),
	// secret=value
	regexp.MustCompile(`(?i)((?:secret[= ]))([A-Za-z0-9_\-\.]{8,})`),
	// AWS/GCP 형식 키 (AKIA... 등 20자 이상)
	regexp.MustCompile(`(AKIA[A-Z0-9]{16,})`),
}

// MaskSensitive는 입력 문자열에서 민감 정보를 ***로 마스킹하여 반환한다.
func MaskSensitive(input string) string {
	result := input
	for _, re := range sensitivePatterns {
		result = re.ReplaceAllStringFunc(result, func(match string) string {
			sub := re.FindStringSubmatch(match)
			if len(sub) == 2 {
				// AWS 키 패턴 (prefix 없음)
				return "***"
			}
			if len(sub) >= 3 {
				return sub[1] + "***"
			}
			return "***"
		})
	}
	return result
}
