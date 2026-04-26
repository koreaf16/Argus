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

const defaultHuggingFaceAPIBase = "https://huggingface.co"

const hfIntentPrompt = `You are an API parameter extractor for HuggingFace Hub. Given a natural language query, output ONLY a JSON object with:
- "keyword": exact model/dataset name or short keyword to search. Empty string if the user wants trending/top/popular without a specific name.
- "sort": one of ["trendingScore","downloads","likes","likes7d","lastModified","createdAt"]. Use "trendingScore" for trending/popular/hot/활발/인기/trade, "downloads" for most downloaded, "likes" for most liked, "lastModified" for recent/latest, "" for relevance.
- "direction": "-1" for descending (default), "1" for ascending.
- "entity": one of ["models","datasets","spaces"]. Default "models". Use "datasets" for dataset queries, "spaces" for app/demo queries.
- "filter": pipeline_tag value like "text-generation","image-to-image","text-to-image","automatic-speech-recognition", or "" if unspecified.
Output ONLY valid JSON, no explanation.`

type HuggingFaceProvider struct {
	httpClient *http.Client
	baseURL    string
	subQuery   ExecutorFn
}

func NewHuggingFaceProvider(httpClient *http.Client, baseURL string, subQuery ExecutorFn) *HuggingFaceProvider {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 12 * time.Second}
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultHuggingFaceAPIBase
	}
	return &HuggingFaceProvider{
		httpClient: httpClient,
		baseURL:    base,
		subQuery:   subQuery,
	}
}

func (p *HuggingFaceProvider) Name() string {
	return "huggingface"
}

func (p *HuggingFaceProvider) Domain() string {
	return "huggingface.co"
}

func (p *HuggingFaceProvider) Match(input SearchInput) ProviderMatch {
	query := normalizedQuery(input.Query)
	score := 0

	if containsDomainHint(input.AllowedDomains, p.Domain()) {
		score = domainMatchScore
	}

	if containsAnyToken(query, []string{"huggingface.co"}) {
		score = maxInt(score, domainMatchScore-10)
	}
	if containsAnyToken(query, []string{"hugging face", "huggingface", "hf.co"}) {
		score = maxInt(score, brandMatchScore)
	}

	if containsAnyToken(query, []string{"model", "models", "model card", "checkpoint", "llm", "dataset", "datasets", "space", "spaces"}) {
		score = maxInt(score, minEntityHintScore)
	}
	return ProviderMatch{Score: score}
}

func (p *HuggingFaceProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	entity := "models"
	keyword := strings.TrimSpace(query)
	sortBy := ""
	direction := ""
	filterTag := ""

	if intent, ok := resolveIntent(ctx, p.subQuery, p.Name(), hfIntentPrompt, query); ok {
		if intent.Entity != "" {
			entity = intent.Entity
		}
		keyword = intent.Keyword
		sortBy = intent.Sort
		direction = intent.Direction
		filterTag = intent.Filter
	} else {
		lowerQuery := normalizedQuery(query)
		if containsAnyToken(lowerQuery, []string{"dataset", "datasets", "데이터셋"}) {
			entity = "datasets"
		} else if containsAnyToken(lowerQuery, []string{"space", "spaces", "스페이스"}) {
			entity = "spaces"
		}
	}

	if maxResults <= 0 {
		maxResults = 10
	}

	endpoint := p.baseURL + "/api/" + entity
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid huggingface endpoint: %w", err)
	}
	qv := u.Query()
	if keyword != "" {
		qv.Set("search", keyword)
	}
	if sortBy != "" {
		qv.Set("sort", sortBy)
	}
	if direction != "" {
		qv.Set("direction", direction)
	}
	if filterTag != "" {
		qv.Set("filter", filterTag)
	}
	qv.Set("limit", strconv.Itoa(maxResults))
	u.RawQuery = qv.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build huggingface request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("huggingface request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("huggingface request failed with status: %d", resp.StatusCode)
	}

	var rows []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, fmt.Errorf("decode huggingface response: %w", err)
	}

	results := make([]SearchResult, 0, len(rows))
	for _, row := range rows {
		repoID := firstString(row, "id", "modelId")
		if repoID == "" {
			continue
		}
		path := repoID
		switch entity {
		case "datasets":
			path = "datasets/" + repoID
		case "spaces":
			path = "spaces/" + repoID
		}

		snippetParts := make([]string, 0, 4)
		snippetParts = appendSnippetText(snippetParts, firstString(row, "pipeline_tag", "library_name", "task", "description"))
		snippetParts = appendSnippetKV(snippetParts, "likes", numberString(row, "likes"))
		snippetParts = appendSnippetKV(snippetParts, "downloads", numberString(row, "downloads"))

		results = append(results, SearchResult{
			Title:   repoID,
			URL:     strings.TrimRight(p.baseURL, "/") + "/" + path,
			Snippet: joinSnippet(snippetParts),
		})
		if len(results) >= maxResults {
			break
		}
	}
	return results, nil
}
