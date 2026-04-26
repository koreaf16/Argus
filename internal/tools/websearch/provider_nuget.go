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

const defaultNuGetSearchBase = "https://azuresearch-usnc.nuget.org/query"

const nugetIntentPrompt = `You are an API parameter extractor for NuGet gallery search. Given a natural language query, output ONLY a JSON object with:
- "keyword": refined package name or short terms. Strip natural language filler words.
- "prerelease": "true" if the user asks for prerelease/preview/beta/alpha packages, "false" otherwise.
- "package_type": one of ["Dependency","DotnetTool","DotnetCliTool","Template",""] based on query context, or "".
Output ONLY valid JSON, no explanation.`

type NuGetProvider struct {
	httpClient *http.Client
	baseURL    string
	subQuery   ExecutorFn
}

func NewNuGetProvider(httpClient *http.Client, baseURL string, subQuery ExecutorFn) *NuGetProvider {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 12 * time.Second}
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultNuGetSearchBase
	}
	return &NuGetProvider{
		httpClient: httpClient,
		baseURL:    base,
		subQuery:   subQuery,
	}
}

func (p *NuGetProvider) Name() string {
	return "nuget"
}

func (p *NuGetProvider) Domain() string {
	return "nuget.org"
}

func (p *NuGetProvider) Match(input SearchInput) ProviderMatch {
	query := normalizedQuery(input.Query)
	score := 0

	if containsDomainHint(input.AllowedDomains, "nuget.org") || containsDomainHint(input.AllowedDomains, "api.nuget.org") {
		score = domainMatchScore
	}
	if containsAnyToken(query, []string{"nuget.org", "api.nuget.org"}) {
		score = maxInt(score, domainMatchScore-10)
	}
	if containsAnyToken(query, []string{"nuget", ".net package", "dotnet package", "c# package", "csharp package"}) {
		score = maxInt(score, brandMatchScore)
	}
	if containsAnyToken(query, []string{"package", "packages", "dependency", "dependencies"}) {
		score = maxInt(score, minEntityHintScore)
	}
	return ProviderMatch{Score: score}
}

func (p *NuGetProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 10
	}

	keyword := strings.TrimSpace(query)
	prerelease := "false"
	packageType := ""

	if intent, ok := resolveIntent(ctx, p.subQuery, p.Name(), nugetIntentPrompt, query); ok {
		if intent.Keyword != "" {
			keyword = intent.Keyword
		}
		if intent.Prerelease == "true" {
			prerelease = "true"
		}
		packageType = intent.PackageType
	}

	u, err := url.Parse(p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid nuget endpoint: %w", err)
	}
	qv := u.Query()
	qv.Set("q", keyword)
	qv.Set("take", strconv.Itoa(maxResults))
	qv.Set("prerelease", prerelease)
	if packageType != "" {
		qv.Set("packageType", packageType)
	}
	u.RawQuery = qv.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build nuget request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("nuget request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nuget request failed with status: %d", resp.StatusCode)
	}

	var payload struct {
		Data []struct {
			ID          string `json:"id"`
			Version     string `json:"version"`
			Description string `json:"description"`
			Total       int64  `json:"totalDownloads"`
			Verified    bool   `json:"verified"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode nuget response: %w", err)
	}

	results := make([]SearchResult, 0, len(payload.Data))
	for _, row := range payload.Data {
		id := strings.TrimSpace(row.ID)
		if id == "" {
			continue
		}
		snippetParts := make([]string, 0, 5)
		snippetParts = appendSnippetText(snippetParts, row.Description)
		snippetParts = appendSnippetKV(snippetParts, "version", row.Version)
		snippetParts = appendSnippetKV(snippetParts, "downloads", strconv.FormatInt(row.Total, 10))
		snippetParts = appendSnippetFlag(snippetParts, "verified", row.Verified)
		results = append(results, SearchResult{
			Title:   id,
			URL:     "https://www.nuget.org/packages/" + url.PathEscape(id),
			Snippet: joinSnippet(snippetParts),
		})
		if len(results) >= maxResults {
			break
		}
	}
	return results, nil
}
