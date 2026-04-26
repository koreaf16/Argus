// Package llm
// File: errors.go
// Description: LLM API 호출 시 발생하는 구조화된 에러 타입과 에러 분류 함수
// Responsibility: HTTP 상태 코드 기반 APIError, 일시적/영구적 에러 분류, Retry-After 파싱

package llm

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// maxErrorBodySize는 진단용 에러 응답 본문 최대 읽기 크기이다.
const maxErrorBodySize = 4096

// APIError는 LLM API 호출에서 HTTP 수준 에러를 운반하는 구조화된 에러 타입이다.
// errors.As를 통해 호출자가 상태 코드와 retry 메타데이터를 검사할 수 있다.
type APIError struct {
	StatusCode int
	RetryAfter time.Duration // Retry-After 헤더에서 파싱; 없으면 zero
	Body       string        // 진단용 응답 본문 (maxErrorBodySize 제한)
	Err        error         // 근본 원인 (nil 가능)
}

// Error는 상태 코드와 본문 발췌를 포함한 에러 메시지를 반환한다.
func (e *APIError) Error() string {
	msg := fmt.Sprintf("api error: status %d", e.StatusCode)
	if e.Body != "" {
		preview := e.Body
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		msg += ": " + preview
	}
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

// Unwrap은 근본 원인 에러를 반환한다.
func (e *APIError) Unwrap() error {
	return e.Err
}

// IsTransient은 에러가 재시도 가능한 일시적 장애인지 판별한다.
// 429(rate limit), 500, 502, 503, 504와 네트워크 타임아웃/연결 리셋을 일시적으로 분류한다.
func IsTransient(err error) bool {
	if err == nil {
		return false
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusTooManyRequests,
			http.StatusInternalServerError,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout:
			return true
		case 529: // Overloaded (Anthropic-specific)
			return true
		}
		return false
	}

	// 네트워크 수준 일시적 에러 판별
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	// 연결 리셋, EOF 등
	msg := err.Error()
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		errors.Is(err, io.ErrUnexpectedEOF)
}

// IsRateLimited는 에러가 HTTP 429 (Too Many Requests)인지 판별한다.
func IsRateLimited(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusTooManyRequests
	}
	return false
}

// IsOverloaded는 에러가 서버 과부하(503, 529)인지 판별한다.
func IsOverloaded(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusServiceUnavailable || apiErr.StatusCode == 529
	}
	return false
}

// RetryAfterDuration은 APIError에 포함된 Retry-After 기간을 반환한다.
// APIError가 아니거나 Retry-After가 없으면 zero를 반환한다.
func RetryAfterDuration(err error) time.Duration {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.RetryAfter
	}
	return 0
}

// parseHTTPError는 2xx가 아닌 HTTP 응답을 *APIError로 변환한다.
// 응답 본문을 maxErrorBodySize까지 읽고, Retry-After 헤더를 파싱한다.
func parseHTTPError(resp *http.Response) *APIError {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))

	retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))

	return &APIError{
		StatusCode: resp.StatusCode,
		RetryAfter: retryAfter,
		Body:       string(body),
	}
}

// parseRetryAfter는 Retry-After 헤더 값을 time.Duration으로 파싱한다.
// 초 단위 정수 또는 HTTP-date 형식을 지원한다. 파싱 실패 시 zero를 반환한다.
func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}

	// 1. 초 단위 정수 시도
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}

	// 2. HTTP-date 형식 시도 (RFC 7231)
	if t, err := http.ParseTime(value); err == nil {
		delta := time.Until(t)
		if delta > 0 {
			return delta
		}
	}

	return 0
}

// IsPTLError는 에러가 prompt-too-long(컨텍스트 길이 초과) 에러인지 판별한다.
// API 에러 body와 일반 에러 메시지 모두에서 PTL 키워드를 탐지한다.
func IsPTLError(err error) bool {
	if err == nil {
		return false
	}

	// APIError의 body에서 탐지
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		lower := strings.ToLower(apiErr.Body)
		if containsPTLKeyword(lower) {
			return true
		}
	}

	// 일반 에러 메시지에서 탐지
	return containsPTLKeyword(strings.ToLower(err.Error()))
}

// containsPTLKeyword는 소문자 문자열에 PTL 관련 키워드가 있는지 확인한다.
func containsPTLKeyword(lower string) bool {
	return strings.Contains(lower, "prompt_too_long") ||
		strings.Contains(lower, "context_length_exceeded") ||
		strings.Contains(lower, "maximum context length") ||
		strings.Contains(lower, "too many tokens")
}
