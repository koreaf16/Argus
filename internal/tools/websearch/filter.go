// Package websearch — domain allow/block filters for search results.
//
// 파일 역할: web_search 입력의 allowed_domains, blocked_domains 옵션을 적용한다.
// 호스트 단위 매칭과 서브도메인 포함 매칭을 지원한다.
// 포함 모듈:
//   - ApplyDomainFilters: 결과 목록에 도메인 필터를 적용한다.
//   - domainMatches: host가 규칙 도메인과 일치하는지 판별한다.
//
// 호출/사용 방식:
//   - internal/tools/websearch/websearch.go 의 Call()에서 DDG 결과 후처리 단계로 호출된다.
//   - 외부 진입점은 ApplyDomainFilters().
//
// 연결:
//   - import 하는 주요 패키지: net/url.
//   - 이 파일을 import 하는 주요 패키지: internal/tools/websearch.
package websearch

import (
	"net/url"
	"strings"
)

func ApplyDomainFilters(results []SearchResult, allowedDomains, blockedDomains []string) []SearchResult {
	allowed := normalizeDomains(allowedDomains)
	blocked := normalizeDomains(blockedDomains)

	if len(allowed) == 0 && len(blocked) == 0 {
		return results
	}

	filtered := make([]SearchResult, 0, len(results))
	for _, result := range results {
		host := normalizedHost(result.URL)
		if host == "" {
			continue
		}
		if len(allowed) > 0 && !matchesAny(host, allowed) {
			continue
		}
		if len(blocked) > 0 && matchesAny(host, blocked) {
			continue
		}
		filtered = append(filtered, result)
	}
	return filtered
}

func normalizeDomains(domains []string) []string {
	if len(domains) == 0 {
		return nil
	}
	out := make([]string, 0, len(domains))
	seen := make(map[string]struct{}, len(domains))
	for _, raw := range domains {
		domain := strings.ToLower(strings.TrimSpace(raw))
		domain = strings.TrimPrefix(domain, "http://")
		domain = strings.TrimPrefix(domain, "https://")
		domain = strings.TrimPrefix(domain, "www.")
		domain = strings.TrimSuffix(domain, "/")
		if domain == "" {
			continue
		}
		if _, exists := seen[domain]; exists {
			continue
		}
		seen[domain] = struct{}{}
		out = append(out, domain)
	}
	return out
}

func normalizedHost(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	host = strings.TrimPrefix(host, "www.")
	return host
}

func matchesAny(host string, rules []string) bool {
	for _, rule := range rules {
		if domainMatches(host, rule) {
			return true
		}
	}
	return false
}

func domainMatches(host, rule string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	rule = strings.ToLower(strings.TrimSpace(rule))
	if host == "" || rule == "" {
		return false
	}
	return host == rule || strings.HasSuffix(host, "."+rule)
}
