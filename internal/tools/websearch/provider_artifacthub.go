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

const defaultArtifactHubBase = "https://artifacthub.io"

const artifacthubIntentPrompt = `You are an API parameter extractor for Artifact Hub search. Given a natural language query, output ONLY a JSON object with:
- "keyword": refined search terms. Strip natural language filler words.
- "sort": "stars" for popular/top/인기, "" or "relevance" for default relevance.
- "kind": package kind as string — "0"=Helm chart, "1"=Falco rules, "2"=OPA policies, "3"=OLM operator, "5"=Krew plugin, "6"=Tekton task, "11"=Container image, ""=all kinds. Infer from context (e.g., "helm chart" → "0", "krew plugin" → "5").
Output ONLY valid JSON, no explanation.`

type ArtifactHubProvider struct {
	httpClient *http.Client
	baseURL    string
	subQuery   ExecutorFn
}

func NewArtifactHubProvider(httpClient *http.Client, baseURL string, subQuery ExecutorFn) *ArtifactHubProvider {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 12 * time.Second}
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultArtifactHubBase
	}
	return &ArtifactHubProvider{
		httpClient: httpClient,
		baseURL:    base,
		subQuery:   subQuery,
	}
}

func (p *ArtifactHubProvider) Name() string {
	return "artifacthub"
}

func (p *ArtifactHubProvider) Domain() string {
	return "artifacthub.io"
}

func (p *ArtifactHubProvider) Match(input SearchInput) ProviderMatch {
	query := normalizedQuery(input.Query)
	score := 0

	if containsDomainHint(input.AllowedDomains, p.Domain()) {
		score = domainMatchScore
	}
	if containsAnyToken(query, []string{"artifacthub.io"}) {
		score = maxInt(score, domainMatchScore-10)
	}
	if containsAnyToken(query, []string{"artifact hub", "artifacthub", "helm chart", "krew plugin", "olm"}) {
		score = maxInt(score, brandMatchScore)
	}
	if containsAnyToken(query, []string{"chart", "charts", "package", "packages", "plugin", "plugins"}) {
		score = maxInt(score, minEntityHintScore)
	}
	return ProviderMatch{Score: score}
}

func (p *ArtifactHubProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 10
	}

	keyword := strings.TrimSpace(query)
	sortBy := ""
	kind := ""

	if intent, ok := resolveIntent(ctx, p.subQuery, p.Name(), artifacthubIntentPrompt, query); ok {
		if intent.Keyword != "" {
			keyword = intent.Keyword
		}
		sortBy = intent.Sort
		kind = intent.Kind
	}

	endpoint := p.baseURL + "/api/v1/packages/search"
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid artifacthub endpoint: %w", err)
	}
	qv := u.Query()
	qv.Set("ts_query_web", keyword)
	qv.Set("limit", strconv.Itoa(maxResults))
	if sortBy != "" && sortBy != "relevance" {
		qv.Set("sort", sortBy)
	}
	if kind != "" {
		qv.Set("kind", kind)
	}
	u.RawQuery = qv.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build artifacthub request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("artifacthub request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("artifacthub request failed with status: %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode artifacthub response: %w", err)
	}

	rows := make([]map[string]any, 0)
	if arr, ok := payload["packages"].([]any); ok {
		for _, item := range arr {
			if row, ok := item.(map[string]any); ok {
				rows = append(rows, row)
			}
		}
	}
	if len(rows) == 0 {
		if arr, ok := payload["data"].([]any); ok {
			for _, item := range arr {
				if row, ok := item.(map[string]any); ok {
					rows = append(rows, row)
				}
			}
		}
	}

	results := make([]SearchResult, 0, len(rows))
	for _, row := range rows {
		name := firstString(row, "name")
		if name == "" {
			continue
		}

		repoName := ""
		if repo, ok := row["repository"].(map[string]any); ok {
			repoName = firstString(repo, "name")
		}
		title := name
		if repoName != "" {
			title = repoName + "/" + name
		}

		urlStr := firstString(row, "url")
		if urlStr == "" {
			urlStr = p.baseURL + "/packages/search?ts_query_web=" + url.QueryEscape(keyword)
		}

		snippetParts := make([]string, 0, 4)
		snippetParts = appendSnippetText(snippetParts, firstString(row, "description"))
		snippetParts = appendSnippetKV(snippetParts, "kind", firstString(row, "kind"))
		snippetParts = appendSnippetKV(snippetParts, "version", firstString(row, "version"))

		results = append(results, SearchResult{
			Title:   title,
			URL:     urlStr,
			Snippet: joinSnippet(snippetParts),
		})
		if len(results) >= maxResults {
			break
		}
	}
	return results, nil
}
