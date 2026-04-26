// Package hooks
// File: timeout.go
// Description: 글로벌 hook 실행 타임아웃 관리
// Responsibility: INFRACTL_HOOK_TIMEOUT 환경변수 읽기 및 기본값 30초 제공

package hooks

import (
	"os"
	"strconv"
	"time"
)

const defaultHookTimeoutSec = 30

// GlobalTimeout 은 환경변수 INFRACTL_HOOK_TIMEOUT (초) 에서 읽은 글로벌 타임아웃이다.
// HookDef.Timeout 이 0 이면 이 값을 사용한다.
func GlobalTimeout() time.Duration {
	if v := os.Getenv("INFRACTL_HOOK_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return defaultHookTimeoutSec * time.Second
}

// HookTimeout 은 HookDef 의 타임아웃을 반환한다.
// HookDef.Timeout 이 0 이면 GlobalTimeout() 을 사용한다.
func HookTimeout(def HookDef) time.Duration {
	if def.Timeout > 0 {
		return time.Duration(def.Timeout * float64(time.Second))
	}
	return GlobalTimeout()
}
