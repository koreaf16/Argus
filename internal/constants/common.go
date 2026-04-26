// Package constants — 날짜·시간 공통 헬퍼.
//
// 파일 역할: 세션 시작 시각 기준 날짜 문자열 반환.
//
//	프롬프트 캐시 안정성을 위해 세션 시작 날짜를 한 번만 계산하여 재사용한다.
//
// 포함 모듈:
//   - GetLocalISODate(): 로컬 날짜를 "YYYY-MM-DD" 형식으로 반환.
//   - GetSessionStartDate(): 세션 시작 시점 날짜를 고정 반환 (once.Do 사용).
//   - GetLocalMonthYear(): "January 2026" 형식 반환.
//
// 호출/사용 방식:
//   - internal/query/context.go 의 시스템 프롬프트 조립 시 참조.
//
// 연결:
//   - 원본: src/constants/common.ts
package constants

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	sessionDateOnce  sync.Once
	sessionStartDate string
)

// GetLocalISODate 는 현재 로컬 날짜를 "YYYY-MM-DD" 형식으로 반환한다.
// ARGUS_OVERRIDE_DATE 환경 변수가 설정되어 있으면 그 값을 반환한다.
func GetLocalISODate() string {
	if override := os.Getenv("ARGUS_OVERRIDE_DATE"); override != "" {
		return override
	}
	now := time.Now()
	return fmt.Sprintf("%04d-%02d-%02d", now.Year(), int(now.Month()), now.Day())
}

// GetSessionStartDate 는 세션 최초 호출 시 날짜를 고정하여 이후 동일 값을 반환한다.
// 프롬프트 캐시 안정성을 위해 세션 도중 날짜가 바뀌어도 재계산하지 않는다.
func GetSessionStartDate() string {
	sessionDateOnce.Do(func() {
		sessionStartDate = GetLocalISODate()
	})
	return sessionStartDate
}

// GetLocalMonthYear 는 "January 2026" 형식의 월-연도 문자열을 반환한다.
// 도구 프롬프트의 캐시 버스팅을 최소화하기 위해 월 단위로만 변경된다.
func GetLocalMonthYear() string {
	if override := os.Getenv("ARGUS_OVERRIDE_DATE"); override != "" {
		t, err := time.Parse("2006-01-02", override)
		if err == nil {
			return t.Format("January 2006")
		}
	}
	return time.Now().Format("January 2006")
}
