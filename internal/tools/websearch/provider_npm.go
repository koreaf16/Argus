package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultNPMRegistryBase = "https://registry.npmjs.org"

const npmIntentPrompt = `You are an API parameter extractor for npm registry search. Given a natural language query, output ONLY a JSON object with:
- "keyword": refined npm search text. Can include npm qualifiers like "keywords:react" or "author:foo". Strip natural language filler words.
- "popularity": float 0-1, weight for popularity ranking. Use 1.0 for "most popular/downloaded/인기", 0.5 default.
- "quality": float 0-1, weight for code quality. Default 0.5.
- "maintenance": float 0-1, weight for maintenance activity. Default 0.5.
Output ONLY valid JSON, no explanation.`

type NPMProvider struct {
	httpClient *http.Client
	baseURL    string
	subQuery   ExecutorFn
}

func NewNPMProvider(httpClient *http.Client, baseURL string, subQuery ExecutorFn) *NPMProvider {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 12 * time.Second}
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultNPMRegistryBase
	}
	return &NPMProvider{
		httpClient: httpClient,
		baseURL:    base,
		subQuery:   subQuery,
	}
}

func (p *NPMProvider) Name() string {
	return "npm"
}

func (p *NPMProvider) Domain() string {
	return "npmjs.com"
}

func (p *NPMProvider) Match(input SearchInput) ProviderMatch {
	query := normalizedQuery(input.Query)
	score := 0

	if containsDomainHint(input.AllowedDomains, "npmjs.com") || containsDomainHint(input.AllowedDomains, "registry.npmjs.org") {
		score = domainMatchScore
	}
	if containsAnyToken(query, []string{"npmjs.com", "registry.npmjs.org"}) {
		score = maxInt(score, domainMatchScore-10)
	}
	if containsAnyToken(query, []string{"npm", "node package", "nodejs package", "javascript package"}) {
		score = maxInt(score, brandMatchScore)
	}
	if containsAnyToken(query, []string{"package", "packages", "module", "modules"}) {
		score = maxInt(score, minEntityHintScore)
	}
	return ProviderMatch{Score: score}
}

func (p *NPMProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 10
	}

	keyword := strings.TrimSpace(query)
	popularity := 0.0
	quality := 0.0
	maintenance := 0.0

	if intent, ok := resolveIntent(ctx, p.subQuery, p.Name(), npmIntentPrompt, query); ok {
		if intent.Keyword != "" {
			keyword = intent.Keyword
		}
		popularity = intent.Popularity
		quality = intent.Quality
		maintenance = intent.Maintenance
	}

	endpoint := p.baseURL + "/-/v1/search"
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid npm endpoint: %w", err)
	}
	qv := u.Query()
	qv.Set("text", keyword)
	qv.Set("size", strconv.Itoa(maxResults))
	if popularity > 0 {
		qv.Set("popularity", strconv.FormatFloat(popularity, 'f', 2, 64))
	}
	if quality > 0 {
		qv.Set("quality", strconv.FormatFloat(quality, 'f', 2, 64))
	}
	if maintenance > 0 {
		qv.Set("maintenance", strconv.FormatFloat(maintenance, 'f', 2, 64))
	}
	u.RawQuery = qv.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build npm request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("npm request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("npm request failed with status: %d", resp.StatusCode)
	}

	var payload struct {
		Objects []struct {
			Package struct {
				Name        string `json:"name"`
				Version     string `json:"version"`
				Description string `json:"description"`
				Links       struct {
					NPM string `json:"npm"`
				} `json:"links"`
			} `json:"package"`
		} `json:"objects"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode npm response: %w", err)
	}

	results := make([]SearchResult, 0, len(payload.Objects))
	for _, item := range payload.Objects {
		name := strings.TrimSpace(item.Package.Name)
		if name == "" {
			continue
		}
		urlStr := strings.TrimSpace(item.Package.Links.NPM)
		if urlStr == "" {
			urlStr = "https://www.npmjs.com/package/" + url.PathEscape(name)
		}
		snippetParts := make([]string, 0, 3)
		snippetParts = appendSnippetText(snippetParts, item.Package.Description)
		snippetParts = appendSnippetKV(snippetParts, "version", item.Package.Version)
		results = append(results, SearchResult{
			Title:   name,
			URL:     urlStr,
			Snippet: joinSnippet(snippetParts),
		})
		if len(results) >= maxResults {
			break
		}
	}
	return results, nil
}
