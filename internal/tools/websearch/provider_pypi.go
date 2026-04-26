package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const defaultPyPIBase = "https://pypi.org"

const pypiIntentPrompt = `You are a Python package name extractor for PyPI. Given a natural language query, output ONLY a JSON object with:
- "keyword": space-separated Python package names to look up. Extract exact package names mentioned. If the query is generic (popular/trending/best for a topic), list the most well-known packages for that topic (e.g., for "web framework" → "fastapi flask django").
Output ONLY valid JSON, no explanation.`

type PyPIProvider struct {
	httpClient *http.Client
	baseURL    string
	subQuery   ExecutorFn
}

func NewPyPIProvider(httpClient *http.Client, baseURL string, subQuery ExecutorFn) *PyPIProvider {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 12 * time.Second}
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultPyPIBase
	}
	return &PyPIProvider{
		httpClient: httpClient,
		baseURL:    base,
		subQuery:   subQuery,
	}
}

func (p *PyPIProvider) Name() string {
	return "pypi"
}

func (p *PyPIProvider) Domain() string {
	return "pypi.org"
}

func (p *PyPIProvider) Match(input SearchInput) ProviderMatch {
	query := normalizedQuery(input.Query)
	score := 0

	if containsDomainHint(input.AllowedDomains, p.Domain()) {
		score = domainMatchScore
	}
	if containsAnyToken(query, []string{"pypi.org"}) {
		score = maxInt(score, domainMatchScore-10)
	}
	if containsAnyToken(query, []string{"pypi", "pip", "python package", "python packages"}) {
		score = maxInt(score, brandMatchScore)
	}
	if containsAnyToken(query, []string{"package", "packages", "wheel", "sdist"}) {
		score = maxInt(score, minEntityHintScore)
	}
	return ProviderMatch{Score: score}
}

func (p *PyPIProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 10
	}

	searchSource := query
	if intent, ok := resolveIntent(ctx, p.subQuery, p.Name(), pypiIntentPrompt, query); ok && intent.Keyword != "" {
		searchSource = intent.Keyword
	}

	candidates := extractPyPICandidates(searchSource, maxResults*2)
	if len(candidates) == 0 {
		return nil, nil
	}

	results := make([]SearchResult, 0, len(candidates))
	for _, pkg := range candidates {
		if len(results) >= maxResults {
			break
		}
		row, err := p.fetchPackage(ctx, pkg)
		if err != nil {
			continue
		}
		results = append(results, row)
	}
	return results, nil
}

func (p *PyPIProvider) fetchPackage(ctx context.Context, pkg string) (SearchResult, error) {
	escaped := url.PathEscape(pkg)
	endpoint := fmt.Sprintf("%s/pypi/%s/json", p.baseURL, escaped)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return SearchResult{}, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return SearchResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return SearchResult{}, fmt.Errorf("package not found")
	}
	if resp.StatusCode != http.StatusOK {
		return SearchResult{}, fmt.Errorf("pypi request failed with status: %d", resp.StatusCode)
	}

	var payload struct {
		Info struct {
			Name    string `json:"name"`
			Summary string `json:"summary"`
			Version string `json:"version"`
			Home    string `json:"home_page"`
		} `json:"info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return SearchResult{}, err
	}

	name := strings.TrimSpace(payload.Info.Name)
	if name == "" {
		name = pkg
	}
	snippetParts := make([]string, 0, 3)
	snippetParts = appendSnippetText(snippetParts, payload.Info.Summary)
	snippetParts = appendSnippetKV(snippetParts, "version", payload.Info.Version)

	return SearchResult{
		Title:   name,
		URL:     fmt.Sprintf("%s/project/%s/", strings.TrimRight(p.baseURL, "/"), url.PathEscape(name)),
		Snippet: joinSnippet(snippetParts),
	}, nil
}

func extractPyPICandidates(query string, limit int) []string {
	original := strings.TrimSpace(query)
	query = strings.ToLower(original)
	replacer := strings.NewReplacer("/", " ", "\\", " ", ",", " ", ":", " ", ";", " ", "\"", " ", "'", " ", "(", " ", ")", " ", "[", " ", "]", " ")
	parts := strings.Fields(replacer.Replace(query))
	if len(parts) == 0 {
		return nil
	}

	stopWords := map[string]struct{}{
		"pypi": {}, "pip": {}, "python": {}, "package": {}, "packages": {}, "latest": {},
		"version": {}, "install": {}, "find": {}, "search": {}, "check": {}, "from": {},
	}

	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	appendCandidate := func(candidate string) {
		candidate = strings.TrimSpace(strings.ToLower(candidate))
		if candidate == "" {
			return
		}
		if _, skip := stopWords[candidate]; skip {
			return
		}
		if !isValidPackageToken(candidate) {
			return
		}
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}

	quoted := regexp.MustCompile("`([^`]+)`|\"([^\"]+)\"|'([^']+)'")
	for _, groups := range quoted.FindAllStringSubmatch(original, -1) {
		for i := 1; i < len(groups); i++ {
			if groups[i] == "" {
				continue
			}
			appendCandidate(groups[i])
		}
	}

	for i := 0; i+2 < len(parts); i++ {
		if parts[i] == "pip" && parts[i+1] == "install" {
			appendCandidate(parts[i+2])
		}
	}

	for _, part := range parts {
		appendCandidate(part)
	}

	for i := 0; i+1 < len(parts); i++ {
		left := strings.TrimSpace(parts[i])
		right := strings.TrimSpace(parts[i+1])
		if left == "" || right == "" {
			continue
		}
		appendCandidate(left + "-" + right)
		appendCandidate(left + "_" + right)
	}

	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func isValidPackageToken(token string) bool {
	if len(token) < 2 {
		return false
	}
	for _, r := range token {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		if r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}
