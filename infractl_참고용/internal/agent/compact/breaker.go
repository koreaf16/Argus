// Package compact
// File: breaker.go
// Description: compaction circuit breaker — 연속 실패 시 자동 차단
// Responsibility: Breaker 인터페이스 정의 및 llm.CircuitBreaker 래핑
//
// Ported from: claude_cli/src/services/compact/autoCompact.ts:70
// MAX_CONSECUTIVE_AUTOCOMPACT_FAILURES = 3
// 기존 코드 이전: internal/agent/agent_struct.go:66 compactBreaker 생성자

package compact

import (
	"time"

	"github.com/yourorg/infractl/internal/llm"
)

// DefaultBreakerThreshold는 circuit breaker 기본 임계값이다.
// Ported from: claude_cli/src/services/compact/autoCompact.ts:70
const DefaultBreakerThreshold = 3

// DefaultBreakerCooldown은 circuit breaker 자동 리셋 기본 대기 시간이다.
const DefaultBreakerCooldown = 5 * time.Minute

// Breaker는 compaction circuit breaker 인터페이스다.
type Breaker interface {
	// Allow는 compaction 시도가 허용되는지 반환한다.
	Allow() bool
	// RecordSuccess는 성공을 기록하고 실패 카운터를 리셋한다.
	RecordSuccess()
	// RecordFailure는 실패를 기록하고 카운터를 증가시킨다.
	RecordFailure()
	// Failures는 현재 연속 실패 횟수를 반환한다.
	Failures() int
}

// NewBreaker는 기본 임계값/쿨다운으로 Breaker를 반환한다.
func NewBreaker() Breaker {
	return NewBreakerWith(DefaultBreakerThreshold, DefaultBreakerCooldown)
}

// NewBreakerWith는 지정 임계값/쿨다운으로 Breaker를 반환한다.
func NewBreakerWith(threshold int, cooldown time.Duration) Breaker {
	return &breakerImpl{cb: llm.NewCircuitBreaker(threshold, cooldown)}
}

type breakerImpl struct {
	cb *llm.CircuitBreaker
}

func (b *breakerImpl) Allow() bool      { return !b.cb.IsOpen() }
func (b *breakerImpl) RecordSuccess()   { b.cb.RecordSuccess() }
func (b *breakerImpl) RecordFailure()   { b.cb.RecordFailure() }
func (b *breakerImpl) Failures() int    { return b.cb.Failures() }

var _ Breaker = (*breakerImpl)(nil)
