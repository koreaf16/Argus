package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type SearXNGResponse struct {
	Query   string `json:"query"`
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	} `json:"results"`
}

type SearXNGClient struct {
	httpClient *http.Client
	endpoint   string
}

func NewSearXNGClient(httpClient *http.Client) *SearXNGClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &SearXNGClient{
		httpClient: httpClient,
	}
}

func (c *SearXNGClient) SetEndpoint(endpoint string) {
	c.endpoint = strings.TrimSpace(endpoint)
}

func (c *SearXNGClient) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return nil, fmt.Errorf("empty search query")
	}
	if strings.TrimSpace(c.endpoint) == "" {
		return nil, fmt.Errorf("empty searxng endpoint")
	}

	u, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid searxng endpoint: %w", err)
	}
	q := u.Query()
	q.Set("q", trimmedQuery)
	q.Set("format", "json")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build searxng request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searxng request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("searxng request failed with status: %d", resp.StatusCode)
	}

	var searXNGResp SearXNGResponse
	if err := json.NewDecoder(resp.Body).Decode(&searXNGResp); err != nil {
		return nil, fmt.Errorf("decode searxng response: %w", err)
	}

	results := make([]SearchResult, 0, len(searXNGResp.Results))
	for i, r := range searXNGResp.Results {
		if maxResults > 0 && i >= maxResults {
			break
		}
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
		})
	}

	return results, nil
}
