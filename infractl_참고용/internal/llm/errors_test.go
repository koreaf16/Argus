package llm

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestAPIError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *APIError
		want string
	}{
		{
			name: "status only",
			err:  &APIError{StatusCode: 429},
			want: "api error: status 429",
		},
		{
			name: "with body",
			err:  &APIError{StatusCode: 500, Body: "internal server error"},
			want: "api error: status 500: internal server error",
		},
		{
			name: "with underlying error",
			err:  &APIError{StatusCode: 503, Err: fmt.Errorf("service unavailable")},
			want: "api error: status 503: service unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAPIError_Unwrap(t *testing.T) {
	inner := fmt.Errorf("root cause")
	apiErr := &APIError{StatusCode: 500, Err: inner}
	if !errors.Is(apiErr, inner) {
		t.Error("Unwrap should expose inner error via errors.Is")
	}
}

func TestIsTransient(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"429", &APIError{StatusCode: 429}, true},
		{"500", &APIError{StatusCode: 500}, true},
		{"502", &APIError{StatusCode: 502}, true},
		{"503", &APIError{StatusCode: 503}, true},
		{"504", &APIError{StatusCode: 504}, true},
		{"529 overloaded", &APIError{StatusCode: 529}, true},
		{"400 bad request", &APIError{StatusCode: 400}, false},
		{"401 unauthorized", &APIError{StatusCode: 401}, false},
		{"403 forbidden", &APIError{StatusCode: 403}, false},
		{"wrapped transient", fmt.Errorf("wrap: %w", &APIError{StatusCode: 429}), true},
		{"wrapped non-transient", fmt.Errorf("wrap: %w", &APIError{StatusCode: 400}), false},
		{"unexpected EOF", io.ErrUnexpectedEOF, true},
		{"connection reset", fmt.Errorf("connection reset by peer"), true},
		{"generic error", fmt.Errorf("something went wrong"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTransient(tt.err); got != tt.want {
				t.Errorf("IsTransient() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsRateLimited(t *testing.T) {
	if !IsRateLimited(&APIError{StatusCode: 429}) {
		t.Error("expected 429 to be rate limited")
	}
	if IsRateLimited(&APIError{StatusCode: 503}) {
		t.Error("expected 503 to not be rate limited")
	}
	if IsRateLimited(fmt.Errorf("generic")) {
		t.Error("expected generic error to not be rate limited")
	}
}

func TestIsOverloaded(t *testing.T) {
	if !IsOverloaded(&APIError{StatusCode: 503}) {
		t.Error("expected 503 to be overloaded")
	}
	if !IsOverloaded(&APIError{StatusCode: 529}) {
		t.Error("expected 529 to be overloaded")
	}
	if IsOverloaded(&APIError{StatusCode: 429}) {
		t.Error("expected 429 to not be overloaded")
	}
}

func TestRetryAfterDuration(t *testing.T) {
	if got := RetryAfterDuration(&APIError{RetryAfter: 5 * time.Second}); got != 5*time.Second {
		t.Errorf("expected 5s, got %v", got)
	}
	if got := RetryAfterDuration(&APIError{}); got != 0 {
		t.Errorf("expected 0, got %v", got)
	}
	if got := RetryAfterDuration(fmt.Errorf("generic")); got != 0 {
		t.Errorf("expected 0 for non-APIError, got %v", got)
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		value string
		want  time.Duration
	}{
		{"", 0},
		{"30", 30 * time.Second},
		{"0", 0},
		{"-1", 0},
		{"invalid", 0},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := parseRetryAfter(tt.value); got != tt.want {
				t.Errorf("parseRetryAfter(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestIsPTLError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"generic", fmt.Errorf("something else"), false},
		{"prompt_too_long in message", fmt.Errorf("prompt_too_long"), true},
		{"context_length_exceeded", fmt.Errorf("context_length_exceeded: max 128000"), true},
		{"maximum context length", fmt.Errorf("maximum context length exceeded"), true},
		{"too many tokens", fmt.Errorf("too many tokens in prompt"), true},
		{"APIError with PTL body", &APIError{StatusCode: 400, Body: `{"error":"prompt_too_long"}`}, true},
		{"APIError without PTL", &APIError{StatusCode: 400, Body: `{"error":"bad request"}`}, false},
		{"wrapped PTL", fmt.Errorf("wrap: %w", fmt.Errorf("context_length_exceeded")), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPTLError(tt.err); got != tt.want {
				t.Errorf("IsPTLError() = %v, want %v", got, tt.want)
			}
		})
	}
}

// timeoutError는 net.Error 인터페이스를 구현하는 테스트용 타입이다.
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

var _ net.Error = (*timeoutError)(nil)

func TestIsTransient_NetworkTimeout(t *testing.T) {
	if !IsTransient(&timeoutError{}) {
		t.Error("expected network timeout to be transient")
	}
}

func TestParseHTTPError(t *testing.T) {
	resp := &http.Response{
		StatusCode: 429,
		Header:     http.Header{"Retry-After": {"60"}},
		Body:       io.NopCloser(io.LimitReader(errReader("rate limited"), 4096)),
	}
	apiErr := parseHTTPError(resp)

	if apiErr.StatusCode != 429 {
		t.Errorf("expected status 429, got %d", apiErr.StatusCode)
	}
	if apiErr.RetryAfter != 60*time.Second {
		t.Errorf("expected RetryAfter 60s, got %v", apiErr.RetryAfter)
	}
}

// errReader는 주어진 문자열을 반환하는 io.Reader이다.
type errReader string

func (r errReader) Read(p []byte) (int, error) {
	n := copy(p, string(r))
	return n, io.EOF
}
