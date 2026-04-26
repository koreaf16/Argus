// Package web
// File: search_hybrid.go
// Description: web 모듈의 기능 수행.
// Responsibility: web 관련 로직 처리 및 관리.

package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SearchAdapterConfig struct {
	GitHub      bool
	HuggingFace bool
	DockerHub   bool
	PyPI        bool
	NPM         bool
	OSV         bool
}

type SearchTokenConfig struct {
	GitHub      string
	HuggingFace string
}

type SearchEndpointConfig struct {
	GitHub      string
	HuggingFace string
	DockerHub   string
	PyPI        string
	NPM         string
	OSV         string
}

type SearchRuntimeConfig struct {
	Timeout   time.Duration
	CacheTTL  time.Duration
	Adapters  SearchAdapterConfig
	Tokens    SearchTokenConfig
	Endpoints SearchEndpointConfig
}

var (
	searchCfgMu sync.RWMutex
	searchCfg   = SearchRuntimeConfig{
		Timeout:  searchTimeout,
		CacheTTL: 2 * time.Minute,
		Adapters: SearchAdapterConfig{
			GitHub:      true,
			HuggingFace: true,
			DockerHub:   true,
			PyPI:        true,
			NPM:         true,
			OSV:         true,
		},
	}

	searchCacheMu sync.Mutex
	searchCache   = map[string]searchCacheEntry{}
)

type searchCacheEntry struct {
	ExpiresAt time.Time
	Results   []SearchResult
}

func ConfigureSearch(cfg SearchRuntimeConfig) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = searchTimeout
	}
	if cfg.CacheTTL < 0 {
		cfg.CacheTTL = 0
	}
	defaultAdapters := SearchAdapterConfig{
		GitHub:      true,
		HuggingFace: true,
		DockerHub:   true,
		PyPI:        true,
		NPM:         true,
		OSV:         true,
	}
	if cfg.Adapters == (SearchAdapterConfig{}) {
		cfg.Adapters = defaultAdapters
	}

	searchCfgMu.Lock()
	searchCfg = cfg
	searchCfgMu.Unlock()
}

func currentSearchConfig() SearchRuntimeConfig {
	searchCfgMu.RLock()
	cfg := searchCfg
	searchCfgMu.RUnlock()
	return cfg
}

func hybridSearch(ctx context.Context, query string, limit int, opts SearchOptions) ([]SearchResult, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	cfg := currentSearchConfig()
	cacheKey := searchCacheKey(q, opts)
	if cached, ok := getCachedSearch(cacheKey, cfg.CacheTTL); ok {
		return trimResults(cached, limit), nil
	}

	adapters := pickAdapters(q, opts, cfg.Adapters)
	var wg sync.WaitGroup

	type adapterResult struct {
		Name    string
		Results []SearchResult
		Err     error
	}

	outCh := make(chan adapterResult, len(adapters)+1)
	for _, a := range adapters {
		a := a
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := a.Search(ctx, q, opts, cfg)
			outCh <- adapterResult{Name: a.Name(), Results: r, Err: err}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		r, err := searchSearXNG(ctx, q, opts, cfg.Timeout)
		outCh <- adapterResult{Name: "searxng", Results: r, Err: err}
	}()

	wg.Wait()
	close(outCh)

	all := make([]SearchResult, 0, 16)
	var fallbackErr error
	var firstAdapterErr error

	for item := range outCh {
		if item.Err != nil {
			if item.Name != "searxng" {
				logAdapterFailure(item.Name, q, item.Err)
			}
			if item.Name == "searxng" {
				fallbackErr = item.Err
			} else if firstAdapterErr == nil {
				firstAdapterErr = item.Err
			}
			continue
		}
		all = append(all, item.Results...)
	}

	if len(all) == 0 {
		if fallbackErr != nil {
			return nil, fallbackErr
		}
		if firstAdapterErr != nil {
			return nil, firstAdapterErr
		}
		return nil, nil
	}

	ranked := rankAndDedupe(all, q, opts)
	if cfg.CacheTTL > 0 {
		setCachedSearch(cacheKey, ranked, cfg.CacheTTL)
	}
	return trimResults(ranked, limit), nil
}

func trimResults(in []SearchResult, limit int) []SearchResult {
	if limit <= 0 || len(in) <= limit {
		return in
	}
	out := make([]SearchResult, 0, limit)
	out = append(out, in[:limit]...)
	return out
}

func searchCacheKey(query string, opts SearchOptions) string {
	allow := normalizeDomainList(opts.AllowedDomains)
	block := normalizeDomainList(opts.BlockedDomains)
	return strings.ToLower(strings.TrimSpace(query)) + "|" + strings.Join(allow, ",") + "|" + strings.Join(block, ",")
}

func getCachedSearch(key string, ttl time.Duration) ([]SearchResult, bool) {
	if ttl <= 0 {
		return nil, false
	}
	searchCacheMu.Lock()
	defer searchCacheMu.Unlock()
	entry, ok := searchCache[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.ExpiresAt) {
		delete(searchCache, key)
		return nil, false
	}
	out := make([]SearchResult, 0, len(entry.Results))
	out = append(out, entry.Results...)
	return out, true
}

func setCachedSearch(key string, results []SearchResult, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	searchCacheMu.Lock()
	searchCache[key] = searchCacheEntry{
		ExpiresAt: time.Now().Add(ttl),
		Results:   append([]SearchResult(nil), results...),
	}
	searchCacheMu.Unlock()
}

type searchAdapter interface {
	Name() string
	Domains() []string
	Search(ctx context.Context, query string, opts SearchOptions, cfg SearchRuntimeConfig) ([]SearchResult, error)
}

type adapterImpl struct {
	name    string
	domains []string
	fn      func(ctx context.Context, query string, opts SearchOptions, cfg SearchRuntimeConfig) ([]SearchResult, error)
}

func (a adapterImpl) Name() string { return a.name }
func (a adapterImpl) Domains() []string {
	return a.domains
}
func (a adapterImpl) Search(ctx context.Context, query string, opts SearchOptions, cfg SearchRuntimeConfig) ([]SearchResult, error) {
	return a.fn(ctx, query, opts, cfg)
}

func pickAdapters(query string, opts SearchOptions, flags SearchAdapterConfig) []searchAdapter {
	q := strings.ToLower(query)
	allowed := normalizeDomainList(opts.AllowedDomains)
	blocked := normalizeDomainList(opts.BlockedDomains)

	includeByDomain := func(domains []string) bool {
		if len(allowed) == 0 {
			return true
		}
		for _, domain := range domains {
			if domainInList(domain, allowed) {
				return true
			}
		}
		return false
	}
	excludedByDomain := func(domains []string) bool {
		for _, domain := range domains {
			if domainInList(domain, blocked) {
				return true
			}
		}
		return false
	}

	looksLikeGitHub := strings.Contains(q, "github") || repoRefPattern.MatchString(q) || hasAny(q, "release", "tag", "pull request", "issue")
	looksLikeHF := hasAny(q, "huggingface", "hf.co", "model", "awq", "gptq", "safetensors")
	looksLikeDocker := hasAny(q, "docker", "container image", "docker hub", "image tag")
	looksLikePyPI := hasAny(q, "pypi", "pip ", "python package")
	looksLikeNPM := hasAny(q, "npm", "node package", "javascript package")
	looksLikeOSV := vulnIDPattern.MatchString(strings.ToUpper(q)) || hasAny(q, "cve", "ghsa", "vulnerability", "advisory")

	adapters := make([]searchAdapter, 0, 6)

	tryAppend := func(enabled bool, looks bool, impl adapterImpl) {
		if !enabled {
			return
		}
		if excludedByDomain(impl.domains) {
			return
		}
		if !includeByDomain(impl.domains) {
			return
		}
		if looks || hasDomainHint(query, impl.domains) {
			adapters = append(adapters, impl)
		}
	}

	tryAppend(flags.GitHub, looksLikeGitHub, adapterImpl{
		name:    "github",
		domains: []string{"github.com"},
		fn:      searchGitHubAdapter,
	})
	tryAppend(flags.HuggingFace, looksLikeHF, adapterImpl{
		name:    "huggingface",
		domains: []string{"huggingface.co"},
		fn:      searchHuggingFaceAdapter,
	})
	tryAppend(flags.DockerHub, looksLikeDocker, adapterImpl{
		name:    "dockerhub",
		domains: []string{"hub.docker.com"},
		fn:      searchDockerHubAdapter,
	})
	tryAppend(flags.PyPI, looksLikePyPI, adapterImpl{
		name:    "pypi",
		domains: []string{"pypi.org"},
		fn:      searchPyPIAdapter,
	})
	tryAppend(flags.NPM, looksLikeNPM, adapterImpl{
		name:    "npm",
		domains: []string{"npmjs.com", "registry.npmjs.org"},
		fn:      searchNPMAdapter,
	})
	tryAppend(flags.OSV, looksLikeOSV, adapterImpl{
		name:    "osv",
		domains: []string{"osv.dev", "api.osv.dev"},
		fn:      searchOSVAdapter,
	})
	return adapters
}

func searchGitHubAdapter(ctx context.Context, query string, _ SearchOptions, cfg SearchRuntimeConfig) ([]SearchResult, error) {
	baseURL := endpointOrDefault(cfg.Endpoints.GitHub, "https://api.github.com/search/repositories")
	reqURL := baseURL + "?q=" + url.QueryEscape(query) + "&per_page=5&sort=stars&order=desc"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)
	if cfg.Tokens.GitHub != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Tokens.GitHub)
	}

	client := newSearXNGHTTPClient(cfg.Timeout)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("github status %d", resp.StatusCode)
	}

	var payload struct {
		Items []struct {
			FullName    string `json:"full_name"`
			HTMLURL     string `json:"html_url"`
			Description string `json:"description"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(payload.Items)+1)
	for _, item := range payload.Items {
		title := strings.TrimSpace(item.FullName)
		rawURL := strings.TrimSpace(item.HTMLURL)
		if title == "" || rawURL == "" {
			continue
		}
		results = append(results, SearchResult{
			Title:      title + " · GitHub",
			URL:        rawURL,
			Snippet:    strings.TrimSpace(item.Description),
			Provider:   "github",
			Kind:       "repository",
			ExactMatch: containsFold(rawURL, extractRepoRef(query)) || containsFold(title, extractRepoRef(query)),
		})
	}

	if hasAny(strings.ToLower(query), "release", "releases", "latest") {
		if repo := extractRepoRef(query); repo != "" {
			results = append(results, SearchResult{
				Title:      "Releases · " + repo + " - GitHub",
				URL:        "https://github.com/" + repo + "/releases",
				Snippet:    "Latest release and tags page.",
				Provider:   "github",
				Kind:       "release",
				ExactMatch: true,
			})
		}
	}
	return results, nil
}

func searchHuggingFaceAdapter(ctx context.Context, query string, _ SearchOptions, cfg SearchRuntimeConfig) ([]SearchResult, error) {
	rawQuery := strings.TrimSpace(query)
	if rawQuery == "" {
		return nil, nil
	}
	queries := []string{rawQuery}
	if fallback := normalizeHuggingFaceSearchQuery(rawQuery); fallback != "" && !strings.EqualFold(fallback, rawQuery) {
		queries = append(queries, fallback)
	}
	for _, q := range queries {
		results, err := searchHuggingFaceOnce(ctx, q, rawQuery, cfg)
		if err != nil {
			return nil, err
		}
		if len(results) > 0 {
			return results, nil
		}
	}
	return nil, nil
}

func searchHuggingFaceOnce(ctx context.Context, searchQuery, originalQuery string, cfg SearchRuntimeConfig) ([]SearchResult, error) {
	baseURL := endpointOrDefault(cfg.Endpoints.HuggingFace, "https://huggingface.co/api/models")
	reqURL := baseURL + "?search=" + url.QueryEscape(searchQuery) + "&limit=5"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	if cfg.Tokens.HuggingFace != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Tokens.HuggingFace)
	}
	client := newSearXNGHTTPClient(cfg.Timeout)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("huggingface status %d", resp.StatusCode)
	}

	var payload []struct {
		ModelID     string `json:"modelId"`
		PipelineTag string `json:"pipeline_tag"`
		Downloads   int    `json:"downloads"`
		Likes       int    `json:"likes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(payload))
	for _, item := range payload {
		modelID := strings.TrimSpace(item.ModelID)
		if modelID == "" {
			continue
		}
		snippet := "Model"
		if item.PipelineTag != "" {
			snippet = "Task: " + item.PipelineTag
		}
		if item.Downloads > 0 {
			snippet += ", downloads=" + strconv.Itoa(item.Downloads)
		}
		results = append(results, SearchResult{
			Title:      modelID + " · Hugging Face",
			URL:        "https://huggingface.co/" + modelID,
			Snippet:    snippet,
			Provider:   "huggingface",
			Kind:       "model",
			ExactMatch: containsFold(modelID, originalQuery) || containsFold(modelID, searchQuery),
		})
	}
	return results, nil
}

func searchDockerHubAdapter(ctx context.Context, query string, _ SearchOptions, cfg SearchRuntimeConfig) ([]SearchResult, error) {
	searchQuery := normalizeDockerHubSearchQuery(query)
	baseURL := endpointOrDefault(cfg.Endpoints.DockerHub, "https://hub.docker.com/v2/search/repositories/")
	reqURL := baseURL + "?query=" + url.QueryEscape(searchQuery) + "&page_size=5"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	client := newSearXNGHTTPClient(cfg.Timeout)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("dockerhub status %d", resp.StatusCode)
	}
	var payload struct {
		Results []struct {
			RepoName         string `json:"repo_name"`
			ShortDescription string `json:"short_description"`
			IsOfficial       bool   `json:"is_official"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(payload.Results))
	for _, item := range payload.Results {
		repo := strings.TrimSpace(item.RepoName)
		if repo == "" {
			continue
		}
		urlVal := "https://hub.docker.com/r/" + repo
		if item.IsOfficial && strings.HasPrefix(repo, "library/") {
			urlVal = "https://hub.docker.com/_/" + strings.TrimPrefix(repo, "library/")
		}
		results = append(results, SearchResult{
			Title:      repo + " - Docker Hub",
			URL:        urlVal,
			Snippet:    strings.TrimSpace(item.ShortDescription),
			Provider:   "dockerhub",
			Kind:       "image",
			ExactMatch: hasAny(strings.ToLower(query), repo, strings.TrimPrefix(repo, "library/")),
		})
	}
	return results, nil
}

func searchPyPIAdapter(ctx context.Context, query string, _ SearchOptions, cfg SearchRuntimeConfig) ([]SearchResult, error) {
	pkg := extractPackageCandidate(query)
	if pkg == "" {
		return nil, nil
	}
	baseURL := endpointOrDefault(cfg.Endpoints.PyPI, "https://pypi.org")
	reqURL := joinURL(baseURL, "/pypi/"+url.PathEscape(pkg)+"/json")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	client := newSearXNGHTTPClient(cfg.Timeout)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("pypi status %d", resp.StatusCode)
	}

	var payload struct {
		Info struct {
			Name    string `json:"name"`
			Summary string `json:"summary"`
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(payload.Info.Name)
	if name == "" {
		return nil, nil
	}
	return []SearchResult{{
		Title:      name + " " + strings.TrimSpace(payload.Info.Version) + " · PyPI",
		URL:        "https://pypi.org/project/" + name + "/",
		Snippet:    strings.TrimSpace(payload.Info.Summary),
		Provider:   "pypi",
		Kind:       "package",
		ExactMatch: strings.EqualFold(name, pkg),
	}}, nil
}

func searchNPMAdapter(ctx context.Context, query string, _ SearchOptions, cfg SearchRuntimeConfig) ([]SearchResult, error) {
	pkg := extractNpmPackageCandidate(query)
	if pkg == "" {
		return nil, nil
	}
	pkgPath := url.PathEscape(pkg)
	pkgPath = strings.ReplaceAll(pkgPath, "%2F", "/")
	baseURL := endpointOrDefault(cfg.Endpoints.NPM, "https://registry.npmjs.org")
	reqURL := joinURL(baseURL, "/"+pkgPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	client := newSearXNGHTTPClient(cfg.Timeout)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("npm status %d", resp.StatusCode)
	}

	var payload struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		DistTags    struct {
			Latest string `json:"latest"`
		} `json:"dist-tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		return nil, nil
	}
	return []SearchResult{{
		Title:      name + " " + strings.TrimSpace(payload.DistTags.Latest) + " · npm",
		URL:        "https://www.npmjs.com/package/" + name,
		Snippet:    strings.TrimSpace(payload.Description),
		Provider:   "npm",
		Kind:       "package",
		ExactMatch: strings.EqualFold(name, pkg),
	}}, nil
}

func searchOSVAdapter(ctx context.Context, query string, _ SearchOptions, cfg SearchRuntimeConfig) ([]SearchResult, error) {
	vulnID := extractVulnerabilityID(query)
	if vulnID == "" {
		return nil, nil
	}
	baseURL := endpointOrDefault(cfg.Endpoints.OSV, "https://api.osv.dev")
	reqURL := joinURL(baseURL, "/v1/vulns/"+url.PathEscape(vulnID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	client := newSearXNGHTTPClient(cfg.Timeout)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("osv status %d", resp.StatusCode)
	}

	var payload struct {
		ID      string   `json:"id"`
		Summary string   `json:"summary"`
		Aliases []string `json:"aliases"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	id := strings.TrimSpace(payload.ID)
	if id == "" {
		return nil, nil
	}
	snippet := strings.TrimSpace(payload.Summary)
	if len(payload.Aliases) > 0 {
		snippet = strings.TrimSpace(snippet + " aliases: " + strings.Join(payload.Aliases, ", "))
	}
	return []SearchResult{{
		Title:      id + " · OSV",
		URL:        "https://osv.dev/vulnerability/" + id,
		Snippet:    snippet,
		Provider:   "osv",
		Kind:       "vulnerability",
		ExactMatch: strings.EqualFold(id, vulnID),
	}}, nil
}

func rankAndDedupe(results []SearchResult, query string, opts SearchOptions) []SearchResult {
	filtered := make([]SearchResult, 0, len(results))
	for _, r := range results {
		if !passesDomainFilters(r.URL, opts) {
			continue
		}
		r.Title = strings.TrimSpace(r.Title)
		r.URL = strings.TrimSpace(r.URL)
		r.Snippet = strings.TrimSpace(r.Snippet)
		if r.URL == "" || r.Title == "" {
			continue
		}
		r.CanonicalURL = canonicalizeURL(r.URL)
		if r.CanonicalURL == "" {
			r.CanonicalURL = r.URL
		}
		score := calculateSourceScore(r.URL, r.Title, r.Provider, r.ExactMatch)
		score += queryMatchBoost(query, r.Title, r.URL)
		r.Score = score
		filtered = append(filtered, r)
	}

	best := map[string]SearchResult{}
	order := make([]string, 0, len(filtered))
	for _, r := range filtered {
		key := strings.ToLower(strings.TrimSpace(r.CanonicalURL))
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(r.URL))
		}
		prev, ok := best[key]
		if !ok {
			best[key] = r
			order = append(order, key)
			continue
		}
		if r.Score > prev.Score {
			best[key] = r
		}
	}

	out := make([]SearchResult, 0, len(best))
	for _, k := range order {
		out = append(out, best[k])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].URL < out[j].URL
		}
		return out[i].Score > out[j].Score
	})
	return out
}

func queryMatchBoost(query, title, rawURL string) float64 {
	terms := queryTerms(query)
	if len(terms) == 0 {
		return 0
	}
	target := strings.ToLower(title + " " + rawURL)
	matches := 0
	for _, term := range terms {
		if strings.Contains(target, term) {
			matches++
		}
	}
	if matches == 0 {
		return 0
	}
	return float64(matches) * 0.35
}

func canonicalizeURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	u.Fragment = ""
	if strings.HasPrefix(strings.ToLower(u.Host), "www.") {
		u.Host = u.Host[4:]
	}
	if (u.Scheme == "http" && u.Host != "localhost") || u.Scheme == "" {
		u.Scheme = "https"
	}
	return u.String()
}

func passesDomainFilters(rawURL string, opts SearchOptions) bool {
	host := normalizeSearchDomain(rawURL)
	if host == "" {
		return false
	}
	allowed := normalizeDomainList(opts.AllowedDomains)
	if len(allowed) > 0 {
		ok := false
		for _, d := range allowed {
			if host == d || strings.HasSuffix(host, "."+d) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	blocked := normalizeDomainList(opts.BlockedDomains)
	for _, d := range blocked {
		if host == d || strings.HasSuffix(host, "."+d) {
			return false
		}
	}
	return true
}

func normalizeDomainList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		d := strings.ToLower(normalizeSearchDomain(raw))
		if d == "" {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

func domainInList(domain string, list []string) bool {
	d := strings.ToLower(normalizeSearchDomain(domain))
	for _, v := range list {
		if d == v || strings.HasSuffix(d, "."+v) || strings.HasSuffix(v, "."+d) {
			return true
		}
	}
	return false
}

func appendDomainHints(query string) string {
	q := strings.TrimSpace(query)
	if q == "" {
		return q
	}
	lower := strings.ToLower(q)
	hints := make([]string, 0, 4)
	appendHint := func(site string) {
		needle := "site:" + site
		if strings.Contains(lower, strings.ToLower(needle)) {
			return
		}
		hints = append(hints, needle)
	}

	switch {
	case hasAny(lower, "maven", "mvn", "gradle dependency"):
		appendHint("search.maven.org")
	case hasAny(lower, "nuget", ".net package"):
		appendHint("nuget.org")
	case hasAny(lower, "crate", "crates.io", "rust package"):
		appendHint("crates.io")
	case hasAny(lower, "terraform", "provider registry", "module registry"):
		appendHint("registry.terraform.io")
	case hasAny(lower, "ansible", "galaxy role"):
		appendHint("galaxy.ansible.com")
	case hasAny(lower, "vscode extension", "visual studio code extension"):
		appendHint("marketplace.visualstudio.com")
	case hasAny(lower, "artifact hub", "helm chart"):
		appendHint("artifacthub.io")
	case hasAny(lower, "gitlab"):
		appendHint("gitlab.com")
	case hasAny(lower, "modelscope"):
		appendHint("modelscope.cn")
	case hasAny(lower, "ollama library", "ollama model"):
		appendHint("ollama.com/library")
	}
	if len(hints) == 0 {
		return q
	}
	return strings.TrimSpace(q + " " + strings.Join(hints, " "))
}

func endpointOrDefault(override, fallback string) string {
	trimmed := strings.TrimSpace(override)
	if trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(fallback)
}

func joinURL(baseURL, path string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	suffix := strings.TrimLeft(strings.TrimSpace(path), "/")
	if base == "" {
		return "/" + suffix
	}
	if suffix == "" {
		return base
	}
	return base + "/" + suffix
}

func normalizeHuggingFaceSearchQuery(query string) string {
	terms := normalizeAdapterSearchQuery(query, map[string]struct{}{
		"huggingface": {}, "hf": {}, "hf.co": {}, "model": {}, "models": {},
		"download": {}, "downloads": {}, "latest": {}, "release": {}, "releases": {},
	})
	if terms == "" {
		return strings.TrimSpace(query)
	}
	return terms
}

func normalizeDockerHubSearchQuery(query string) string {
	terms := normalizeAdapterSearchQuery(query, map[string]struct{}{
		"docker": {}, "dockerhub": {}, "hub": {}, "image": {}, "images": {},
		"container": {}, "containers": {}, "tag": {}, "tags": {},
		"official": {}, "latest": {},
	})
	if terms == "" {
		return strings.TrimSpace(query)
	}
	return terms
}

func normalizeAdapterSearchQuery(query string, stop map[string]struct{}) string {
	tokens := wordPattern.FindAllString(strings.ToLower(query), -1)
	if len(tokens) == 0 {
		return ""
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if strings.HasPrefix(token, "site:") {
			continue
		}
		if _, ok := stop[token]; ok {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
		if len(out) == 5 {
			break
		}
	}
	return strings.TrimSpace(strings.Join(out, " "))
}

func hasDomainHint(query string, domains []string) bool {
	lower := strings.ToLower(query)
	for _, d := range domains {
		if strings.Contains(lower, strings.ToLower(d)) {
			return true
		}
	}
	return false
}

func hasAny(text string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(text, strings.ToLower(term)) {
			return true
		}
	}
	return false
}

var (
	repoRefPattern = regexp.MustCompile(`\b[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+\b`)
	vulnIDPattern  = regexp.MustCompile(`\b(?:CVE-\d{4}-\d{4,}|GHSA-[0-9A-Za-z]{4}-[0-9A-Za-z]{4}-[0-9A-Za-z]{4})\b`)
	wordPattern    = regexp.MustCompile(`[A-Za-z0-9@._/-]+`)
)

func extractRepoRef(query string) string {
	return strings.TrimSpace(repoRefPattern.FindString(query))
}

func extractVulnerabilityID(query string) string {
	return strings.ToUpper(strings.TrimSpace(vulnIDPattern.FindString(strings.ToUpper(query))))
}

func queryTerms(query string) []string {
	stop := map[string]struct{}{
		"latest": {}, "release": {}, "releases": {}, "official": {}, "docs": {}, "documentation": {},
		"install": {}, "guide": {}, "site": {}, "version": {}, "package": {}, "model": {},
		"github": {}, "huggingface": {}, "docker": {}, "hub": {},
	}
	raw := wordPattern.FindAllString(strings.ToLower(query), -1)
	seen := map[string]struct{}{}
	out := make([]string, 0, len(raw))
	for _, token := range raw {
		token = strings.TrimSpace(token)
		if len(token) < 3 {
			continue
		}
		if _, ok := stop[token]; ok {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	return out
}

func extractPackageCandidate(query string) string {
	lower := strings.ToLower(query)
	if !hasAny(lower, "pypi", "pip", "python package") {
		return ""
	}
	tokens := wordPattern.FindAllString(query, -1)
	for _, token := range tokens {
		t := strings.TrimSpace(token)
		if t == "" {
			continue
		}
		l := strings.ToLower(t)
		if hasAny(l, "pypi", "pip", "python", "package", "latest", "version") {
			continue
		}
		if repoRefPattern.MatchString(t) {
			continue
		}
		return t
	}
	return ""
}

func extractNpmPackageCandidate(query string) string {
	lower := strings.ToLower(query)
	if !hasAny(lower, "npm", "node package", "javascript package") {
		return ""
	}
	tokens := wordPattern.FindAllString(query, -1)
	for _, token := range tokens {
		t := strings.TrimSpace(token)
		if t == "" {
			continue
		}
		l := strings.ToLower(t)
		if hasAny(l, "npm", "node", "javascript", "package", "latest", "version") {
			continue
		}
		if strings.Contains(t, "/") && !strings.HasPrefix(t, "@") {
			continue
		}
		return t
	}
	return ""
}

func containsFold(text, sub string) bool {
	if strings.TrimSpace(sub) == "" {
		return false
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(sub))
}

func logAdapterFailure(name, query string, err error) {
	slog.Debug("search adapter failed", "adapter", name, "query", query, "err", err)
}
