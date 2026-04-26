package llm

import (
	"testing"
	"time"
)

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.BaseDelay != 500*time.Millisecond {
		t.Errorf("BaseDelay = %v, want 500ms", cfg.BaseDelay)
	}
	if cfg.MaxDelay != 32*time.Second {
		t.Errorf("MaxDelay = %v, want 32s", cfg.MaxDelay)
	}
}

func TestRetryDelay_ExponentialBackoff(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:     5,
		BaseDelay:      1 * time.Second,
		MaxDelay:       60 * time.Second,
		JitterFraction: 0, // 지터 비활성화로 결정적 테스트
	}

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 1 * time.Second},  // 1s * 2^0 = 1s
		{2, 2 * time.Second},  // 1s * 2^1 = 2s
		{3, 4 * time.Second},  // 1s * 2^2 = 4s
		{4, 8 * time.Second},  // 1s * 2^3 = 8s
		{5, 16 * time.Second}, // 1s * 2^4 = 16s
	}

	for _, tt := range tests {
		got := RetryDelay(tt.attempt, cfg, nil)
		if got != tt.want {
			t.Errorf("RetryDelay(attempt=%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestRetryDelay_MaxDelayCap(t *testing.T) {
	cfg := RetryConfig{
		BaseDelay:      1 * time.Second,
		MaxDelay:       5 * time.Second,
		JitterFraction: 0,
	}

	got := RetryDelay(10, cfg, nil) // 1s * 2^9 = 512s, capped at 5s
	if got != 5*time.Second {
		t.Errorf("RetryDelay(attempt=10) = %v, want 5s (capped)", got)
	}
}

func TestRetryDelay_JitterAddsPositive(t *testing.T) {
	cfg := RetryConfig{
		BaseDelay:      1 * time.Second,
		MaxDelay:       60 * time.Second,
		JitterFraction: 0.5,
	}

	// 지터가 있으면 base delay보다 크거나 같아야 한다
	for i := 0; i < 100; i++ {
		got := RetryDelay(1, cfg, nil)
		if got < 1*time.Second {
			t.Errorf("RetryDelay with jitter = %v, should be >= 1s", got)
		}
		// 지터 최대: 0.5 * 1s = 500ms
		if got > 1500*time.Millisecond {
			t.Errorf("RetryDelay with jitter = %v, should be <= 1.5s", got)
		}
	}
}

func TestRetryDelay_RetryAfterOverride(t *testing.T) {
	cfg := RetryConfig{
		BaseDelay:      500 * time.Millisecond,
		MaxDelay:       60 * time.Second,
		JitterFraction: 0,
	}

	// RetryAfter가 계산값보다 크면 RetryAfter를 사용
	err := &APIError{StatusCode: 429, RetryAfter: 10 * time.Second}
	got := RetryDelay(1, cfg, err) // 계산값: 500ms, RetryAfter: 10s
	if got != 10*time.Second {
		t.Errorf("RetryDelay with RetryAfter = %v, want 10s", got)
	}
}

func TestRetryDelay_ZeroAttempt(t *testing.T) {
	cfg := RetryConfig{
		BaseDelay:      1 * time.Second,
		MaxDelay:       60 * time.Second,
		JitterFraction: 0,
	}

	// attempt < 1은 1로 처리
	got := RetryDelay(0, cfg, nil)
	if got != 1*time.Second {
		t.Errorf("RetryDelay(attempt=0) = %v, want 1s", got)
	}
}

func TestCircuitBreaker_BasicFlow(t *testing.T) {
	cb := NewCircuitBreaker(3, 5*time.Minute)

	// 초기 상태: closed
	if cb.IsOpen() {
		t.Error("expected circuit breaker to be closed initially")
	}

	// 2번 실패: 아직 closed
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.IsOpen() {
		t.Error("expected circuit breaker to be closed after 2 failures")
	}
	if cb.Failures() != 2 {
		t.Errorf("Failures() = %d, want 2", cb.Failures())
	}

	// 3번째 실패: open
	cb.RecordFailure()
	if !cb.IsOpen() {
		t.Error("expected circuit breaker to be open after 3 failures")
	}

	// 성공 기록: closed
	cb.RecordSuccess()
	if cb.IsOpen() {
		t.Error("expected circuit breaker to be closed after success")
	}
	if cb.Failures() != 0 {
		t.Errorf("Failures() = %d, want 0 after success", cb.Failures())
	}
}

func TestCircuitBreaker_AutoReset(t *testing.T) {
	// resetAfter를 매우 짧게 설정하여 즉시 리셋 테스트
	cb := NewCircuitBreaker(1, 1*time.Millisecond)
	cb.RecordFailure()

	if !cb.IsOpen() {
		t.Error("expected open after failure")
	}

	// 리셋 대기
	time.Sleep(5 * time.Millisecond)

	if cb.IsOpen() {
		t.Error("expected auto-reset after resetAfter duration")
	}
}
