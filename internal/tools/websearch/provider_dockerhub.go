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

const defaultDockerHubAPIBase = "https://hub.docker.com"

const dockerhubIntentPrompt = `You are an API parameter extractor for Docker Hub search. Given a natural language query, output ONLY a JSON object with:
- "keyword": refined image name or short keyword. Strip natural language filler words like "find", "search", "latest", "popular".
- "filter": one of ["official","automated",""]. Use "official" if the query asks for official images, "" otherwise.
Output ONLY valid JSON, no explanation.`

type DockerHubProvider struct {
	httpClient *http.Client
	baseURL    string
	subQuery   ExecutorFn
}

func NewDockerHubProvider(httpClient *http.Client, baseURL string, subQuery ExecutorFn) *DockerHubProvider {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 12 * time.Second}
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultDockerHubAPIBase
	}
	return &DockerHubProvider{
		httpClient: httpClient,
		baseURL:    base,
		subQuery:   subQuery,
	}
}

func (p *DockerHubProvider) Name() string {
	return "dockerhub"
}

func (p *DockerHubProvider) Domain() string {
	return "hub.docker.com"
}

func (p *DockerHubProvider) Match(input SearchInput) ProviderMatch {
	query := normalizedQuery(input.Query)
	score := 0

	if containsDomainHint(input.AllowedDomains, p.Domain()) || containsDomainHint(input.AllowedDomains, "docker.com") {
		score = domainMatchScore
	}
	if containsAnyToken(query, []string{"hub.docker.com", "docker.com"}) {
		score = maxInt(score, domainMatchScore-10)
	}
	if containsAnyToken(query, []string{"docker hub", "dockerhub", "docker"}) {
		score = maxInt(score, brandMatchScore)
	}
	if containsAnyToken(query, []string{"image", "container", "tag"}) {
		score = maxInt(score, minEntityHintScore)
	}
	return ProviderMatch{Score: score}
}

func (p *DockerHubProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 10
	}

	keyword := strings.TrimSpace(query)
	filterType := ""

	if intent, ok := resolveIntent(ctx, p.subQuery, p.Name(), dockerhubIntentPrompt, query); ok {
		if intent.Keyword != "" {
			keyword = intent.Keyword
		}
		filterType = intent.Filter
	}

	endpoint := p.baseURL + "/v2/search/repositories/"
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid docker hub endpoint: %w", err)
	}
	qv := u.Query()
	qv.Set("query", keyword)
	qv.Set("page_size", strconv.Itoa(maxResults))
	if filterType != "" {
		qv.Set("image_filter", filterType)
	}
	u.RawQuery = qv.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build docker hub request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("docker hub request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("docker hub request failed with status: %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode docker hub response: %w", err)
	}

	rows := make([]map[string]any, 0)
	if arr, ok := payload["results"].([]any); ok {
		for _, item := range arr {
			if row, ok := item.(map[string]any); ok {
				rows = append(rows, row)
			}
		}
	}
	if len(rows) == 0 {
		if arr, ok := payload["summaries"].([]any); ok {
			for _, item := range arr {
				if row, ok := item.(map[string]any); ok {
					rows = append(rows, row)
				}
			}
		}
	}

	results := make([]SearchResult, 0, len(rows))
	for _, row := range rows {
		repo := firstString(row, "repo_name", "name", "slug")
		if repo == "" {
			continue
		}
		if !strings.Contains(repo, "/") {
			repo = "library/" + repo
		}

		snippetParts := make([]string, 0, 4)
		snippetParts = appendSnippetText(snippetParts, firstString(row, "short_description", "description"))
		snippetParts = appendSnippetKV(snippetParts, "stars", numberString(row, "star_count"))
		snippetParts = appendSnippetKV(snippetParts, "pulls", numberString(row, "pull_count"))

		results = append(results, SearchResult{
			Title:   repo,
			URL:     "https://hub.docker.com/r/" + repo,
			Snippet: joinSnippet(snippetParts),
		})
		if len(results) >= maxResults {
			break
		}
	}
	return results, nil
}
