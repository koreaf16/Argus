package llm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	maxRetryAttempts = 3
	retryBaseDelay   = time.Second
	retryMaxDelay    = 16 * time.Second
)

// doWithRetry 는 HTTP 요청을 재시도 정책에 따라 실행한다.
//
// 재시도 조건:
//   - 네트워크 오류 (일시적인 연결 실패)
//   - 429 Too Many Requests: Retry-After 헤더 또는 backoff 대기
//   - 500/502/503/504: exponential backoff 후 재시도
//
// ctx 취소 시 즉시 중단한다.
func doWithRetry(ctx context.Context, client *http.Client, makeReq func() (*http.Request, error)) (*http.Response, error) {
	delay := retryBaseDelay
	var lastErr error

	for attempt := 0; attempt <= maxRetryAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			delay = min(delay*2, retryMaxDelay)
		}

		req, err := makeReq()
		if err != nil {
			return nil, err
		}

		resp, err := client.Do(req)
		if err != nil {
			// 네트워크 오류는 재시도
			lastErr = err
			continue
		}

		switch resp.StatusCode {
		case http.StatusTooManyRequests: // 429
			// Retry-After 헤더 우선 적용
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, parseErr := strconv.Atoi(ra); parseErr == nil && secs > 0 {
					delay = time.Duration(secs) * time.Second
					if delay > retryMaxDelay {
						delay = retryMaxDelay
					}
				}
			}
			_ = drainAndClose(resp.Body)
			lastErr = fmt.Errorf("rate limited (429)")
			continue

		case http.StatusInternalServerError, // 500
			http.StatusBadGateway,         // 502
			http.StatusServiceUnavailable, // 503
			http.StatusGatewayTimeout:     // 504
			_ = drainAndClose(resp.Body)
			lastErr = fmt.Errorf("server error (%d)", resp.StatusCode)
			continue

		default:
			// 2xx 또는 4xx(재시도 무의미) → 호출자에게 그대로 반환
			return resp, nil
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("request failed after %d retries: %w", maxRetryAttempts, lastErr)
	}
	return nil, fmt.Errorf("request failed after %d retries", maxRetryAttempts)
}

func drainAndClose(body io.ReadCloser) error {
	if body == nil {
		return nil
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 4096))
	return body.Close()
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
