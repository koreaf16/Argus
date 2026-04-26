// Package compact
// File: breaker_test.go
// Description: circuit breaker 동작 단위 테스트
// Responsibility: 3회 연속 실패 → open, 성공 → 리셋 검증

package compact

import (
	"testing"
	"time"
)

func TestBreaker_OpenAfterThresholdFailures(t *testing.T) {
	b := NewBreakerWith(DefaultBreakerThreshold, 1*time.Hour)

	for i := range DefaultBreakerThreshold {
		if !b.Allow() {
			t.Fatalf("breaker should be closed before threshold (iteration %d)", i)
		}
		b.RecordFailure()
	}

	if b.Allow() {
		t.Error("breaker should be open after threshold failures")
	}
}

func TestBreaker_SuccessResetsFailures(t *testing.T) {
	b := NewBreakerWith(DefaultBreakerThreshold, 1*time.Hour)

	for range DefaultBreakerThreshold - 1 {
		b.RecordFailure()
	}

	b.RecordSuccess()

	if b.Failures() != 0 {
		t.Errorf("RecordSuccess should reset failures, got %d", b.Failures())
	}
	if !b.Allow() {
		t.Error("breaker should be closed after success")
	}
}

func TestBreaker_CooldownReopens(t *testing.T) {
	b := NewBreakerWith(1, 1*time.Millisecond)
	b.RecordFailure()

	if b.Allow() {
		t.Error("breaker should be open immediately after failure")
	}

	time.Sleep(5 * time.Millisecond)

	if !b.Allow() {
		t.Error("breaker should allow after cooldown")
	}
}
