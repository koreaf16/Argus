// Package shell — bash 출력 길이 상수 및 환경변수 기반 조회 함수.
//
// 파일 역할: 최대 출력 길이 상수를 정의하고, BASH_MAX_OUTPUT_LENGTH 환경변수를
//
//	읽어 유효 범위로 clamp 한 값을 반환한다.
//
// 포함 모듈:
//   - BashMaxOutputUpperLimit: 절대 상한 (150_000)
//   - BashMaxOutputDefault: 기본값 (30_000)
//   - GetMaxOutputLength: env 변수를 읽어 clamp 된 길이를 반환
//
// 호출/사용 방식:
//   - internal/tools/bash 에서 명령 출력 길이를 제한할 때 호출
//
// 연결:
//   - 원본: src/utils/shell/outputLimits.ts
package shell

import (
	"os"
	"strconv"
)

const (
	// BashMaxOutputUpperLimit 은 BASH_MAX_OUTPUT_LENGTH 의 절대 상한이다.
	BashMaxOutputUpperLimit = 150_000
	// BashMaxOutputDefault 는 환경변수가 미설정이거나 유효하지 않을 때 사용하는 기본값이다.
	BashMaxOutputDefault = 30_000
)

// GetMaxOutputLength 는 BASH_MAX_OUTPUT_LENGTH 환경변수를 읽어 [1, BashMaxOutputUpperLimit]
// 범위로 clamp 한 값을 반환한다. 미설정이거나 파싱 불가능하면 BashMaxOutputDefault 를 반환한다.
func GetMaxOutputLength() int {
	raw := os.Getenv("BASH_MAX_OUTPUT_LENGTH")
	if raw == "" {
		return BashMaxOutputDefault
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return BashMaxOutputDefault
	}
	if n < 1 {
		return 1
	}
	if n > BashMaxOutputUpperLimit {
		return BashMaxOutputUpperLimit
	}
	return n
}
