package websearch

import (
	"context"
	"net/http"
	"strings"
)

const (
	minEntityHintScore = 40
	domainMatchScore   = 120
	brandMatchScore    = 90
)

type ProviderMatch struct {
	Score int
}

type SearchProvider interface {
	Name() string
	Domain() string
	Match(input SearchInput) ProviderMatch
	Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error)
}

type SearchRouter struct {
	providers []SearchProvider
}

func NewSearchRouter(providers ...SearchProvider) *SearchRouter {
	out := make([]SearchProvider, 0, len(providers))
	for _, p := range providers {
		if p == nil {
			continue
		}
		out = append(out, p)
	}
	return &SearchRouter{providers: out}
}

func NewDefaultSearchRouter(httpClient *http.Client, subQuery ExecutorFn) *SearchRouter {
	return NewSearchRouter(
		NewHuggingFaceProvider(httpClient, "", subQuery),
		NewGitHubProvider(httpClient, "", subQuery),
		NewDockerHubProvider(httpClient, "", subQuery),
		NewPyPIProvider(httpClient, "", subQuery),
		NewNPMProvider(httpClient, "", subQuery),
		NewMavenCentralProvider(httpClient, "", subQuery),
		NewNuGetProvider(httpClient, "", subQuery),
		NewArtifactHubProvider(httpClient, "", subQuery),
	)
}

// Route returns the best-matching provider for the input, or nil if no strong match exists.
func (r *SearchRouter) Route(input SearchInput) SearchProvider {
	p, _ := r.RouteWithScore(input)
	return p
}

// RouteWithScore returns the best-matching provider and its score.
func (r *SearchRouter) RouteWithScore(input SearchInput) (SearchProvider, int) {
	if r == nil || len(r.providers) == 0 {
		return nil, 0
	}

	bestScore := 0
	var best SearchProvider
	tie := false

	for _, provider := range r.providers {
		if provider == nil {
			continue
		}
		if !isProviderAllowed(provider, input.AllowedDomains) {
			continue
		}
		if isProviderBlocked(provider, input.BlockedDomains) {
			continue
		}
		match := provider.Match(input)
		if match.Score <= 0 {
			continue
		}
		if match.Score > bestScore {
			bestScore = match.Score
			best = provider
			tie = false
			continue
		}
		if match.Score == bestScore {
			tie = true
		}
	}

	if tie || bestScore <= 0 {
		return nil, 0
	}
	return best, bestScore
}

func isProviderAllowed(provider SearchProvider, allowedDomains []string) bool {
	allowed := normalizeDomains(allowedDomains)
	if len(allowed) == 0 || provider == nil {
		return true
	}
	domain := strings.ToLower(strings.TrimSpace(provider.Domain()))
	if domain == "" {
		return false
	}
	for _, allowedDomain := range allowed {
		if domainMatches(domain, allowedDomain) || domainMatches(allowedDomain, domain) {
			return true
		}
	}
	return false
}

func isProviderBlocked(provider SearchProvider, blockedDomains []string) bool {
	if provider == nil {
		return false
	}
	domain := strings.ToLower(strings.TrimSpace(provider.Domain()))
	if domain == "" {
		return false
	}
	for _, blocked := range normalizeDomains(blockedDomains) {
		if domainMatches(domain, blocked) {
			return true
		}
	}
	return false
}

func normalizedQuery(query string) string {
	return strings.ToLower(strings.TrimSpace(query))
}

func containsAnyToken(text string, tokens []string) bool {
	if text == "" || len(tokens) == 0 {
		return false
	}
	for _, token := range tokens {
		token = strings.TrimSpace(strings.ToLower(token))
		if token == "" {
			continue
		}
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func containsDomainHint(hints []string, domain string) bool {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return false
	}
	for _, hint := range normalizeDomains(hints) {
		if domainMatches(domain, hint) {
			return true
		}
	}
	return false
}
