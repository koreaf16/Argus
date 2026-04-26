// Package hooks
// File: backend.go
// Description: Hook backend 인터페이스 정의
// Responsibility: 소비자(runner)가 사용하는 Backend 인터페이스 단일 정의

package hooks

import "context"

// Backend 는 단일 hook 을 실행하는 실행기이다.
// command / prompt / http / agent 네 가지 구현이 있다.
type Backend interface {
	// Run 은 hook 을 실행하고 결과를 반환한다.
	// ctx 는 timeout 이 적용된 context 여야 한다.
	Run(ctx context.Context, def HookDef, input HookInput) (HookOutput, error)
}
