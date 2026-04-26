// Package llm
// File: retry.go
// Description: LLM API 호출 재시도를 위한 지수 백오프, 지터, circuit breaker 인프라
// Responsibility: RetryConfig 관리, 재시도 지연 계산, 연속 실패 차단

package llm

import (
	"math"
	"math/rand/v2"
	"sync"
	"time"
)

// RetryConfig는 LLM API 호출 재시도 동작을 제어한다.
type RetryConfig struct {
	MaxRetries     int           // 최대 재시도 횟수 (첫 시도 제외)
	BaseDelay      time.Duration // 첫 재시도 기본 대기 시간
	MaxDelay       time.Duration // 대기 시간 상한
	JitterFraction float64       // 기본 지연 대비 지터 비율 (0.0~1.0)
}

// DefaultRetryConfig는 일반적인 LLM API 호출에 적합한 기본 재시도 설정을 반환한다.
// 500ms → 1s → 2s (지터 포함), 최대 3회 재시도.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     3,
		BaseDelay:      500 * time.Millisecond,
		MaxDelay:       32 * time.Second,
		JitterFraction: 0.25,
	}
}

// RetryDelay는 주어진 시도 횟수와 에러에 기반하여 다음 재시도까지 대기 시간을 계산한다.
//
// 계산 방식:
//   - 기본: min(baseDelay * 2^(attempt-1), maxDelay)
//   - 지터: + random [0, jitterFraction * baseDelay)
//   - RetryAfter 우선: 에러에 Retry-After가 있으면 해당 값과 계산값 중 큰 것 사용
func RetryDelay(attempt int, cfg RetryConfig, err error) time.Duration {
	if attempt < 1 {
		attempt = 1
	}

	// 지수 백오프: baseDelay * 2^(attempt-1)
	exp := math.Pow(2, float64(attempt-1))
	delay := time.Duration(float64(cfg.BaseDelay) * exp)
	if delay > cfg.MaxDelay {
		delay = cfg.MaxDelay
	}

	// 지터 추가: [0, jitterFraction * baseDelay)
	if cfg.JitterFraction > 0 {
		jitterMax := float64(cfg.BaseDelay) * cfg.JitterFraction
		jitter := time.Duration(rand.Float64() * jitterMax)
		delay += jitter
	}

	// Retry-After 헤더 우선: 서버 지시가 계산값보다 크면 서버 지시를 따른다
	if ra := RetryAfterDuration(err); ra > delay {
		delay = ra
	}

	return delay
}

// CircuitBreaker는 연속 실패를 추적하여 임계값 초과 시 호출을 차단한다.
// 일정 시간 동안 호출이 없으면 자동으로 half-open 상태로 리셋된다.
type CircuitBreaker struct {
	mu          sync.Mutex
	threshold   int
	failures    int
	lastFailure time.Time
	resetAfter  time.Duration
}

// NewCircuitBreaker는 주어진 임계값과 자동 리셋 기간으로 CircuitBreaker를 생성한다.
func NewCircuitBreaker(threshold int, resetAfter time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold:  threshold,
		resetAfter: resetAfter,
	}
}

// RecordFailure는 실패를 기록하고 연속 실패 카운터를 증가시킨다.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	cb.lastFailure = time.Now()
}

// RecordSuccess는 연속 실패 카운터를 0으로 리셋한다.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
}

// IsOpen은 circuit breaker가 열려 있는지 (호출 차단 상태) 반환한다.
// 연속 실패가 임계값 이상이고, 마지막 실패로부터 resetAfter가 지나지 않았으면 true.
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.failures < cb.threshold {
		return false
	}

	// 자동 리셋: resetAfter 경과 시 half-open으로 전환
	if cb.resetAfter > 0 && time.Since(cb.lastFailure) >= cb.resetAfter {
		cb.failures = 0
		return false
	}

	return true
}

// Failures는 현재 연속 실패 횟수를 반환한다.
func (cb *CircuitBreaker) Failures() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.failures
}
