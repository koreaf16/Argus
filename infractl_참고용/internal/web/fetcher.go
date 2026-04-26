// Package web
// File: fetcher.go
// Description: URL 콘텐츠 HTTP 페처 (LRU 캐시 + HTML→Markdown 변환)
// Responsibility: URL 기반 웹 콘텐츠 가져오기, 캐싱, LLM 2단계 처리 지원

package web

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/yourorg/infractl/internal/cache"
	"golang.org/x/net/html/charset"
)

const (
	fetchTimeout = 15 * time.Second
	maxBodyBytes = 5 * 1024 * 1024 // 5MB
	fetchMaxLen  = 100000
	userAgent    = "infractl/1.0 (infrastructure management agent)"
)

// Fetcher는 URL 내용을 가져와 Markdown으로 변환하는 HTTP 클라이언트이다.
type Fetcher struct {
	client *http.Client
	cache  *cache.Cache[string, string]
}

// NewFetcher는 크기 기반 LRU 캐시를 포함한 Fetcher를 생성한다.
// cacheSize, cacheTTL 파라미터는 하위 호환성을 위해 유지되나 내부적으로 기본 설정을 사용한다.
func NewFetcher(_ int, _ time.Duration) *Fetcher {
	return &Fetcher{
		client: &http.Client{Timeout: fetchTimeout},
		cache:  cache.NewWebCache(),
	}
}

// Fetch는 URL 내용을 가져와 Markdown으로 반환한다.
// 동일 URL 재요청 시 캐시에서 반환한다.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (string, error) {
	// 캐시 확인
	if cached, ok := f.cache.Get(rawURL); ok {
		slog.Debug("web cache hit", "url", rawURL)
		return cached, nil
	}

	body, err := f.httpGet(ctx, rawURL)
	if err != nil {
		return "", err
	}

	md := HTMLToMarkdown(body, fetchMaxLen)
	f.cache.Set(rawURL, md)
	slog.Debug("web fetch complete", "url", rawURL, "markdown_len", len(md))
	return md, nil
}

// FetchWithPrompt는 URL 내용을 가져와 Markdown으로 변환한 뒤,
// promptFn(markdown)을 호출하여 LLM 처리 결과를 반환한다.
// promptFn이 nil이면 Fetch와 동일하게 동작한다.
func (f *Fetcher) FetchWithPrompt(ctx context.Context, rawURL string, promptFn func(ctx context.Context, markdown string) (string, error)) (string, error) {
	md, err := f.Fetch(ctx, rawURL)
	if err != nil {
		return "", err
	}
	if promptFn == nil {
		return md, nil
	}
	result, err := promptFn(ctx, md)
	if err != nil {
		slog.Warn("web fetch prompt fn failed, returning raw markdown", "url", rawURL, "err", err)
		return md, nil
	}
	return result, nil
}

// httpGet은 HTTP GET으로 URL 내용을 반환한다. (자동 인코딩 변환 포함)
func (f *Fetcher) httpGet(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request %s: %w", rawURL, err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %d for %s", resp.StatusCode, rawURL)
	}

	// 인코딩 자동 감지 및 변환 리더 생성 (UTF-8로 변환)
	utf8Reader, err := charset.NewReader(resp.Body, resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, fmt.Errorf("create charset reader %s: %w", rawURL, err)
	}

	body, err := io.ReadAll(io.LimitReader(utf8Reader, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("read body %s: %w", rawURL, err)
	}
	return body, nil
}
