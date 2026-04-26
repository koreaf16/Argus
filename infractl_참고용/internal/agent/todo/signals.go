// Package todo
// File: signals.go
// Description: 다단계 작업 신호어 사전 및 감지 함수
// Responsibility: 사용자 입력에서 다단계 작업 신호어 감지

package todo

import "strings"

// multiStepSignals 는 다단계 작업을 암시하는 신호어 목록이다.
var multiStepSignals = []string{
	// 한국어
	"설치", "배포", "마이그레이션", "업그레이드", "롤백",
	"구성", "설정", "프로비저닝", "이관", "이전",
	// 영문
	"install", "deploy", "migrate", "migration", "upgrade",
	"rollback", "provision", "setup", "configure", "bootstrap",
}

// DetectMultiStep 은 input 에 다단계 신호어가 포함되어 있으면 true 를 반환한다.
func DetectMultiStep(input string) bool {
	lower := strings.ToLower(input)
	for _, sig := range multiStepSignals {
		if strings.Contains(lower, sig) {
			return true
		}
	}
	return false
}
