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

const defaultGitHubAPIBase = "https://api.github.com"

const githubIntentPrompt = `You are an API parameter extractor for GitHub Search API. Given a natural language query, output ONLY a JSON object with:
- "keyword": refined search terms. Include GitHub query qualifiers if appropriate (e.g., "language:python stars:>100", "topic:machine-learning"). Strip natural language filler words.
- "sort": one of ["stars","forks","updated",""]. Use "stars" for popular/top/best/인기, "updated" for recent/latest/최신, "" for relevance.
- "direction": "desc" (default) or "asc".
Output ONLY valid JSON, no explanation.`

type GitHubProvider struct {
	httpClient *http.Client
	baseURL    string
	subQuery   ExecutorFn
}

func NewGitHubProvider(httpClient *http.Client, baseURL string, subQuery ExecutorFn) *GitHubProvider {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 12 * time.Second}
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultGitHubAPIBase
	}
	return &GitHubProvider{
		httpClient: httpClient,
		baseURL:    base,
		subQuery:   subQuery,
	}
}

func (p *GitHubProvider) Name() string {
	return "github"
}

func (p *GitHubProvider) Domain() string {
	return "github.com"
}

func (p *GitHubProvider) Match(input SearchInput) ProviderMatch {
	query := normalizedQuery(input.Query)
	score := 0

	if containsDomainHint(input.AllowedDomains, p.Domain()) {
		score = domainMatchScore
	}
	if containsAnyToken(query, []string{"github.com"}) {
		score = maxInt(score, domainMatchScore-10)
	}
	if containsAnyToken(query, []string{"github", "git hub"}) {
		score = maxInt(score, brandMatchScore)
	}
	if containsAnyToken(query, []string{"repo", "repository"}) {
		score = maxInt(score, minEntityHintScore)
	}
	return ProviderMatch{Score: score}
}

func (p *GitHubProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 10
	}

	keyword := strings.TrimSpace(query)
	sortBy := ""
	direction := "desc"

	if intent, ok := resolveIntent(ctx, p.subQuery, p.Name(), githubIntentPrompt, query); ok {
		if intent.Keyword != "" {
			keyword = intent.Keyword
		}
		sortBy = intent.Sort
		if intent.Direction != "" {
			direction = intent.Direction
		}
	}

	endpoint := p.baseURL + "/search/repositories"
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid github endpoint: %w", err)
	}
	qv := u.Query()
	qv.Set("q", keyword)
	qv.Set("per_page", strconv.Itoa(maxResults))
	if sortBy != "" {
		qv.Set("sort", sortBy)
		qv.Set("order", direction)
	}
	u.RawQuery = qv.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build github request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github request failed with status: %d", resp.StatusCode)
	}

	var payload struct {
		Items []struct {
			FullName string `json:"full_name"`
			HTMLURL  string `json:"html_url"`
			Desc     string `json:"description"`
			Stars    int    `json:"stargazers_count"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode github response: %w", err)
	}

	results := make([]SearchResult, 0, len(payload.Items))
	for _, item := range payload.Items {
		if strings.TrimSpace(item.FullName) == "" || strings.TrimSpace(item.HTMLURL) == "" {
			continue
		}
		snippet := strings.TrimSpace(item.Desc)
		if item.Stars > 0 {
			if snippet != "" {
				snippet += ", "
			}
			snippet += "stars=" + strconv.Itoa(item.Stars)
		}
		results = append(results, SearchResult{
			Title:   item.FullName,
			URL:     item.HTMLURL,
			Snippet: snippet,
		})
		if len(results) >= maxResults {
			break
		}
	}
	return results, nil
}
