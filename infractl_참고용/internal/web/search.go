// Package web
// File: search.go
// Description: SearXNG JSON API 웹 검색 클라이언트
// Responsibility: SearXNG 인스턴스에 질의하여 검색 결과 반환

package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
)

const (
	SearXNGBaseURL = "http://192.168.0.3:30080"
	searchTimeout  = 10 * time.Second
	maxResults     = 10
)

// SearchResult는 웹 검색 결과 단일 항목이다.
type SearchResult struct {
	Title        string
	URL          string
	Snippet      string
	Provider     string
	Kind         string
	CanonicalURL string
	ExactMatch   bool
	Score        float64
}

// SearchOptions customizes a web search request.
type SearchOptions struct {
	AllowedDomains []string
	BlockedDomains []string
}

// SearchOption mutates SearchOptions.
type SearchOption func(*SearchOptions)

// WithAllowedDomains constrains results to the given domains.
func WithAllowedDomains(domains ...string) SearchOption {
	return func(opts *SearchOptions) {
		opts.AllowedDomains = append(opts.AllowedDomains, domains...)
	}
}

// WithBlockedDomains excludes results from the given domains.
func WithBlockedDomains(domains ...string) SearchOption {
	return func(opts *SearchOptions) {
		opts.BlockedDomains = append(opts.BlockedDomains, domains...)
	}
}

// Search runs the hybrid search flow:
// provider adapters first, SearXNG fallback, normalized merge/rank.
func Search(ctx context.Context, query string, limit int, opts ...SearchOption) ([]SearchResult, error) {
	if limit <= 0 || limit > maxResults {
		limit = 5
	}

	searchOpts := SearchOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&searchOpts)
		}
	}
	return hybridSearch(ctx, query, limit, searchOpts)
}

// RawSearch bypasses the hybrid adapter path and queries SearXNG directly.
// Used by web_search inner loops when generic query reformulation is preferred.
func RawSearch(ctx context.Context, query string, limit int, opts ...SearchOption) ([]SearchResult, error) {
	if limit <= 0 || limit > maxResults {
		limit = 5
	}

	searchOpts := SearchOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&searchOpts)
		}
	}

	results, err := searchSearXNG(ctx, query, searchOpts, searchTimeout)
	if err != nil {
		return nil, err
	}
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func newSearXNGHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = searchTimeout
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: nil,
		},
	}
}

type searxngResponse struct {
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	} `json:"results"`
}

func calculateSourceScore(rawURL, title, provider string, exactMatch bool) float64 {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0
	}
	host := strings.ToLower(strings.TrimSpace(u.Host))

	officialDomains := map[string]float64{
		"docs.vllm.ai":                 5.0,
		"huggingface.co":               4.8,
		"hub.docker.com":               4.8,
		"pypi.org":                     4.7,
		"registry.npmjs.org":           4.7,
		"npmjs.com":                    4.7,
		"osv.dev":                      4.8,
		"api.osv.dev":                  4.8,
		"github.com":                   4.8,
		"gitlab.com":                   4.4,
		"artifacthub.io":               4.6,
		"registry.terraform.io":        4.6,
		"galaxy.ansible.com":           4.4,
		"marketplace.visualstudio.com": 4.5,
		"ai.google.dev":                4.5,
		"blog.google":                  4.0,
		"nvidia.com":                   4.0,
		"developer.nvidia.com":         4.5,
		"pytorch.org":                  4.0,
		"tensorflow.org":               4.0,
		"kubernetes.io":                4.0,
		"oracle.com":                   3.0,
		"arxiv.org":                    3.5,
		"search.maven.org":             4.6,
		"nuget.org":                    4.6,
		"crates.io":                    4.6,
		"docs.github.com":              4.8,
		"docs.gitlab.com":              4.4,
	}

	score := 0.0
	for domain, s := range officialDomains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			score = s
			break
		}
	}

	lowerTitle := strings.ToLower(title)
	if strings.Contains(lowerTitle, "official") || strings.Contains(lowerTitle, "documentation") {
		score += 1.0
	}
	if provider != "" {
		score += 1.5
	}
	if exactMatch {
		score += 2.5
	}

	blogDomains := []string{
		"medium.com",
		"dev.to",
		"blogspot.com",
		"wordpress.com",
		"tistory.com",
		"velog.io",
	}
	for _, domain := range blogDomains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			score -= 1.5
			break
		}
	}

	return score
}

func searchSearXNG(ctx context.Context, query string, opts SearchOptions, timeout time.Duration) ([]SearchResult, error) {
	searchQuery := buildSearchQuery(appendDomainHints(query), opts)

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	params := url.Values{
		"q":      {searchQuery},
		"format": {"json"},
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, SearXNGBaseURL+"/search", strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("searxng search request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := newSearXNGHTTPClient(timeout)
	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("searxng search failed", "query", searchQuery, "err", err)
		return nil, fmt.Errorf("searxng unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("searxng search error status", "query", searchQuery, "status", resp.Status)
		return nil, fmt.Errorf("searxng returned status %s (check if JSON format is enabled)", resp.Status)
	}

	// 인코딩 자동 감지 및 변환 리더 생성 (UTF-8로 변환)
	utf8Reader, err := charset.NewReader(resp.Body, resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, fmt.Errorf("create charset reader: %w", err)
	}

	body, err := io.ReadAll(io.LimitReader(utf8Reader, 512*1024))
	if err != nil {
		slog.Warn("searxng read body failed", "err", err)
		return nil, nil
	}

	var parsed searxngResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		slog.Warn("searxng parse failed", "err", err)
		return nil, nil
	}

	results := make([]SearchResult, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		title := strings.TrimSpace(r.Title)
		rawURL := strings.TrimSpace(r.URL)
		if title == "" || rawURL == "" {
			continue
		}
		results = append(results, SearchResult{
			Title:   title,
			URL:     rawURL,
			Snippet: strings.TrimSpace(r.Content),
			Kind:    "web",
		})
	}

	slog.Debug("searxng search done", "query", searchQuery, "results", len(results))
	return results, nil
}

func buildSearchQuery(query string, opts SearchOptions) string {
	parts := []string{strings.TrimSpace(query)}
	seen := make(map[string]bool)

	appendDomainClause := func(prefix, raw string) {
		domain := normalizeSearchDomain(raw)
		if domain == "" {
			return
		}
		clause := prefix + domain
		if seen[clause] {
			return
		}
		seen[clause] = true
		parts = append(parts, clause)
	}

	for _, domain := range opts.AllowedDomains {
		appendDomainClause("site:", domain)
	}
	for _, domain := range opts.BlockedDomains {
		appendDomainClause("-site:", domain)
	}

	return strings.TrimSpace(strings.Join(parts, " "))
}

func normalizeSearchDomain(raw string) string {
	domain := strings.TrimSpace(raw)
	if domain == "" {
		return ""
	}
	domain = strings.TrimPrefix(domain, "site:")
	domain = strings.TrimPrefix(domain, "-site:")
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "www.")
	domain = strings.Trim(domain, "/")
	if domain == "" {
		return ""
	}
	if strings.Contains(domain, "/") {
		domain = strings.SplitN(domain, "/", 2)[0]
	}
	return domain
}
