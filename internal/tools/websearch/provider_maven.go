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

const defaultMavenCentralBase = "https://search.maven.org"

const mavenIntentPrompt = `You are an API parameter extractor for Maven Central Solr search. Given a natural language query, output ONLY a JSON object with:
- "keyword": Maven search query using Solr syntax. Use "g:<groupId>" for group filter, "a:<artifactId>" for artifact filter (e.g., "g:org.springframework a:spring-core"), or free keyword text for general search. Extract groupId/artifactId if mentioned in the query.
Output ONLY valid JSON, no explanation.`

type MavenCentralProvider struct {
	httpClient *http.Client
	baseURL    string
	subQuery   ExecutorFn
}

func NewMavenCentralProvider(httpClient *http.Client, baseURL string, subQuery ExecutorFn) *MavenCentralProvider {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 12 * time.Second}
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultMavenCentralBase
	}
	return &MavenCentralProvider{
		httpClient: httpClient,
		baseURL:    base,
		subQuery:   subQuery,
	}
}

func (p *MavenCentralProvider) Name() string {
	return "mavencentral"
}

func (p *MavenCentralProvider) Domain() string {
	return "search.maven.org"
}

func (p *MavenCentralProvider) Match(input SearchInput) ProviderMatch {
	query := normalizedQuery(input.Query)
	score := 0

	if containsDomainHint(input.AllowedDomains, "search.maven.org") || containsDomainHint(input.AllowedDomains, "central.sonatype.org") {
		score = domainMatchScore
	}
	if containsAnyToken(query, []string{"search.maven.org", "central.sonatype.org"}) {
		score = maxInt(score, domainMatchScore-10)
	}
	if containsAnyToken(query, []string{"maven", "maven central", "gradle", "artifact"}) {
		score = maxInt(score, brandMatchScore)
	}
	if containsAnyToken(query, []string{"jar", "dependency", "dependencies", "groupid", "artifactid"}) {
		score = maxInt(score, minEntityHintScore)
	}
	return ProviderMatch{Score: score}
}

func (p *MavenCentralProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 10
	}

	keyword := strings.TrimSpace(query)
	if intent, ok := resolveIntent(ctx, p.subQuery, p.Name(), mavenIntentPrompt, query); ok && intent.Keyword != "" {
		keyword = intent.Keyword
	}

	endpoint := p.baseURL + "/solrsearch/select"
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid maven endpoint: %w", err)
	}
	qv := u.Query()
	qv.Set("q", keyword)
	qv.Set("rows", strconv.Itoa(maxResults))
	qv.Set("wt", "json")
	u.RawQuery = qv.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build maven request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("maven request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("maven request failed with status: %d", resp.StatusCode)
	}

	var payload struct {
		Response struct {
			Docs []struct {
				Group         string `json:"g"`
				Artifact      string `json:"a"`
				LatestVersion string `json:"latestVersion"`
				Packaging     string `json:"p"`
			} `json:"docs"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode maven response: %w", err)
	}

	results := make([]SearchResult, 0, len(payload.Response.Docs))
	for _, doc := range payload.Response.Docs {
		group := strings.TrimSpace(doc.Group)
		artifact := strings.TrimSpace(doc.Artifact)
		if group == "" || artifact == "" {
			continue
		}
		version := strings.TrimSpace(doc.LatestVersion)
		title := group + ":" + artifact
		snippetParts := make([]string, 0, 3)
		snippetParts = appendSnippetKV(snippetParts, "latestVersion", version)
		snippetParts = appendSnippetKV(snippetParts, "packaging", doc.Packaging)

		searchQuery := fmt.Sprintf("g:%q AND a:%q", group, artifact)
		urlStr := "https://search.maven.org/search?q=" + url.QueryEscape(searchQuery)
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
