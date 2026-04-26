package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestRetryWithTransientThenSuccess는 처음 N회 503을 반환한 후 200을 반환하는
// 서버를 시뮬레이션하여 ChatStream의 재시도 기반 호출 패턴을 검증한다.
// 이 테스트는 LLM 클라이언트 수준의 retry가 아닌, retry를 수행하는 호출자(agent loop)의
// 패턴을 단위 검증하기 위한 것이다.
func TestRetryWithTransientThenSuccess(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n <= 2 {
			// 처음 2회: 503 Service Unavailable
			w.WriteHeader(503)
			fmt.Fprint(w, `{"error":"overloaded"}`)
			return
		}
		// 3회째: 정상 응답
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"choices":[{"message":{"content":"success","tool_calls":null}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`)
	}))
	defer srv.Close()

	client := NewOpenAIClient(srv.URL, "test-model", "", 5*time.Second)
	cfg := RetryConfig{
		MaxRetries:     3,
		BaseDelay:      10 * time.Millisecond, // 테스트용 짧은 지연
		MaxDelay:       100 * time.Millisecond,
		JitterFraction: 0,
	}
	ctx := context.Background()

	// agent loop의 retry 패턴 시뮬레이션
	var resp Response
	var lastErr error
	for attempt := 1; attempt <= cfg.MaxRetries+1; attempt++ {
		resp, lastErr = client.Chat(ctx, []Message{{Role: RoleUser, Content: "test"}}, nil, nil)
		if lastErr == nil {
			break
		}
		if !IsTransient(lastErr) {
			t.Fatalf("expected transient error, got: %v", lastErr)
		}
		if attempt > cfg.MaxRetries {
			t.Fatalf("all retries exhausted: %v", lastErr)
		}
		delay := RetryDelay(attempt, cfg, lastErr)
		time.Sleep(delay)
	}

	if lastErr != nil {
		t.Fatalf("expected success after retries, got: %v", lastErr)
	}
	if resp.Content != "success" {
		t.Errorf("Content = %q, want 'success'", resp.Content)
	}
	if callCount.Load() != 3 {
		t.Errorf("server called %d times, want 3", callCount.Load())
	}
}

// TestRetryWithPermanentErrorBailsOut은 400 Bad Request처럼 비일시적 에러가 발생하면
// 재시도 없이 즉시 종료되는지 검증한다.
func TestRetryWithPermanentErrorBailsOut(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(400)
		fmt.Fprint(w, `{"error":"bad request"}`)
	}))
	defer srv.Close()

	client := NewOpenAIClient(srv.URL, "test-model", "", 5*time.Second)
	cfg := DefaultRetryConfig()
	cfg.BaseDelay = 10 * time.Millisecond
	ctx := context.Background()

	var lastErr error
	for attempt := 1; attempt <= cfg.MaxRetries+1; attempt++ {
		_, lastErr = client.Chat(ctx, []Message{{Role: RoleUser, Content: "test"}}, nil, nil)
		if lastErr == nil {
			t.Fatal("expected error for 400 response")
		}
		if !IsTransient(lastErr) {
			break // 비일시적 에러: 즉시 중단
		}
		if attempt > cfg.MaxRetries {
			break
		}
		time.Sleep(RetryDelay(attempt, cfg, lastErr))
	}

	if callCount.Load() != 1 {
		t.Errorf("server called %d times, want 1 (no retry for 400)", callCount.Load())
	}
	if IsTransient(lastErr) {
		t.Error("400 should not be transient")
	}
}

// TestRetryRespectsRetryAfterHeader는 429 응답의 Retry-After 헤더가
// RetryDelay 계산에 반영되는지 검증한다.
func TestRetryRespectsRetryAfterHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(429)
		fmt.Fprint(w, `{"error":"rate limited"}`)
	}))
	defer srv.Close()

	client := NewOpenAIClient(srv.URL, "test-model", "", 5*time.Second)
	_, err := client.Chat(context.Background(), []Message{{Role: RoleUser, Content: "test"}}, nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}

	cfg := RetryConfig{
		BaseDelay:      100 * time.Millisecond,
		MaxDelay:       60 * time.Second,
		JitterFraction: 0,
	}

	delay := RetryDelay(1, cfg, err)
	// Retry-After=2s는 BaseDelay(100ms)보다 크므로 2s를 사용해야 한다
	if delay < 2*time.Second {
		t.Errorf("RetryDelay = %v, want >= 2s (from Retry-After header)", delay)
	}
}

// TestRetryWithRateLimitThenRecovery는 429→429→200 시나리오를 검증한다.
func TestRetryWithRateLimitThenRecovery(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n <= 2 {
			w.Header().Set("Retry-After", "0") // 즉시 재시도 허용
			w.WriteHeader(429)
			fmt.Fprint(w, `{"error":"rate limited"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"choices":[{"message":{"content":"recovered"}}]}`)
	}))
	defer srv.Close()

	client := NewOpenAIClient(srv.URL, "test-model", "", 5*time.Second)
	cfg := RetryConfig{
		MaxRetries:     3,
		BaseDelay:      5 * time.Millisecond,
		MaxDelay:       50 * time.Millisecond,
		JitterFraction: 0,
	}
	ctx := context.Background()

	var resp Response
	var lastErr error
	for attempt := 1; attempt <= cfg.MaxRetries+1; attempt++ {
		resp, lastErr = client.Chat(ctx, []Message{{Role: RoleUser, Content: "test"}}, nil, nil)
		if lastErr == nil {
			break
		}
		if !IsTransient(lastErr) {
			t.Fatalf("expected transient, got: %v", lastErr)
		}
		if attempt > cfg.MaxRetries {
			t.Fatal("retries exhausted")
		}
		time.Sleep(RetryDelay(attempt, cfg, lastErr))
	}

	if lastErr != nil {
		t.Fatalf("expected recovery, got: %v", lastErr)
	}
	if resp.Content != "recovered" {
		t.Errorf("Content = %q, want 'recovered'", resp.Content)
	}
	if callCount.Load() != 3 {
		t.Errorf("calls = %d, want 3", callCount.Load())
	}
}

// TestCircuitBreakerPreventsRetryAfterThreshold는 연속 실패가
// circuit breaker 임계값을 넘으면 추가 호출을 차단하는지 검증한다.
func TestCircuitBreakerPreventsRetryAfterThreshold(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(503)
		fmt.Fprint(w, `overloaded`)
	}))
	defer srv.Close()

	client := NewOpenAIClient(srv.URL, "test-model", "", 5*time.Second)
	cb := NewCircuitBreaker(2, 1*time.Minute)
	cfg := RetryConfig{
		MaxRetries:     1,
		BaseDelay:      5 * time.Millisecond,
		MaxDelay:       10 * time.Millisecond,
		JitterFraction: 0,
	}
	ctx := context.Background()

	// 루프 1: retry 소진 → breaker에 실패 기록
	_, err := retryCall(ctx, client, cfg, cb)
	if err == nil {
		t.Fatal("expected error")
	}

	// 루프 2: retry 소진 → breaker에 실패 기록 (2번째)
	_, err = retryCall(ctx, client, cfg, cb)
	if err == nil {
		t.Fatal("expected error")
	}

	// 이제 breaker가 열려야 한다
	if !cb.IsOpen() {
		t.Fatal("circuit breaker should be open after 2 failures")
	}

	// 루프 3: breaker가 열려 있으므로 호출 시도 안 함
	callsBefore := callCount.Load()
	if !cb.IsOpen() {
		t.Fatal("breaker should still be open")
	}
	// 실제 호출을 하지 않았으므로 카운트가 변하지 않아야 한다
	if callCount.Load() != callsBefore {
		t.Error("no additional calls should have been made with open breaker")
	}
}

// retryCall은 agent loop의 retry 패턴을 시뮬레이션하는 헬퍼이다.
func retryCall(ctx context.Context, client *OpenAIClient, cfg RetryConfig, cb *CircuitBreaker) (Response, error) {
	var resp Response
	var lastErr error
	for attempt := 1; attempt <= cfg.MaxRetries+1; attempt++ {
		resp, lastErr = client.Chat(ctx, []Message{{Role: RoleUser, Content: "test"}}, nil, nil)
		if lastErr == nil {
			cb.RecordSuccess()
			return resp, nil
		}
		if !IsTransient(lastErr) {
			return resp, lastErr
		}
		if attempt > cfg.MaxRetries {
			cb.RecordFailure()
			return resp, lastErr
		}
		time.Sleep(RetryDelay(attempt, cfg, lastErr))
	}
	return resp, lastErr
}
