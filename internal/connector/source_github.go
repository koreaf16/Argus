package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type GitHubSource struct {
	cache *Cache
}

func NewGitHubSource(cache *Cache) *GitHubSource {
	return &GitHubSource{cache: cache}
}

func (s *GitHubSource) Search(ctx context.Context, query string) ([]ConnectorSpec, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	// If it looks like a direct github.com/owner/repo reference, don't include in general search
	if strings.HasPrefix(query, "github.com/") {
		return nil, nil
	}

	cacheKey := "github-search-" + query
	if cached, ok := s.cache.Get(cacheKey, 30*time.Minute); ok {
		var specs []ConnectorSpec
		if err := json.Unmarshal(cached, &specs); err == nil {
			return specs, nil
		}
	}

	// Search GitHub for MCP server repos matching query
	q := url.QueryEscape(query + " mcp server in:name,description,topics")
	apiURL := fmt.Sprintf("https://api.github.com/search/repositories?q=%s&sort=stars&order=desc&per_page=10", q)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return nil, nil // Rate limited — silently skip GitHub results
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Items []struct {
			Name        string `json:"name"`
			FullName    string `json:"full_name"`
			Description string `json:"description"`
			Stars       int    `json:"stargazers_count"`
			Topics      []string `json:"topics"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	var specs []ConnectorSpec
	for _, item := range payload.Items {
		spec := ConnectorSpec{
			Name:        item.FullName,
			Description: item.Description,
			Source:      "github",
			SourceURL:   "github.com/" + item.FullName,
			Runtime:     RuntimeNPX,
			Command:     "npx",
			Args:        []string{"-y", "github:" + item.FullName},
		}
		specs = append(specs, spec)
	}

	if b, err := json.Marshal(specs); err == nil {
		_ = s.cache.Set(cacheKey, b)
	}

	return specs, nil
}

func (s *GitHubSource) Info(ctx context.Context, name string) (*ConnectorSpec, error) {
	if !strings.HasPrefix(name, "github.com/") {
		return nil, fmt.Errorf("not a github repository")
	}

	// github.com/owner/repo → owner/repo
	repoPath := strings.TrimPrefix(name, "github.com/")
	parts := strings.SplitN(repoPath, "/", 2)
	if len(parts) < 2 || parts[1] == "" {
		return nil, fmt.Errorf("invalid github path: %s", name)
	}

	runtime, err := s.detectRuntime(ctx, repoPath)
	if err != nil {
		runtime = RuntimeNPX // fallback
	}

	envPrompts, _ := s.extractEnvPrompts(ctx, repoPath)

	spec := ConnectorSpec{
		Name:        parts[1], // use repo name as connector name
		Description: fmt.Sprintf("Connector installed from %s", name),
		Source:      "github",
		SourceURL:   name,
		Runtime:     runtime,
		EnvPrompts:  envPrompts,
	}

	switch runtime {
	case RuntimeNPX:
		spec.Command = "npx"
		spec.Args = []string{"-y", "github:" + repoPath}
	case RuntimeUVX:
		spec.Command = "uvx"
		spec.Args = []string{"--from", "git+https://github.com/" + repoPath, parts[1]}
	case RuntimeGo:
		spec.Command = "go"
		spec.Args = []string{"run", "github.com/" + repoPath + "@latest"}
	case RuntimeDocker:
		spec.Command = "docker"
		spec.Args = []string{"run", "--rm", "-i", "ghcr.io/" + repoPath}
	default:
		spec.Runtime = RuntimeNPX
		spec.Command = "npx"
		spec.Args = []string{"-y", "github:" + repoPath}
	}

	return &spec, nil
}

// detectRuntime fetches the repository contents to determine the runtime.
func (s *GitHubSource) detectRuntime(ctx context.Context, repoPath string) (RuntimeType, error) {
	checks := []struct {
		file    string
		runtime RuntimeType
	}{
		{"package.json", RuntimeNPX},
		{"pyproject.toml", RuntimeUVX},
		{"setup.py", RuntimeUVX},
		{"go.mod", RuntimeGo},
		{"Dockerfile", RuntimeDocker},
	}

	for _, check := range checks {
		apiURL := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", repoPath, check.file)
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("Accept", "application/vnd.github+json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return check.runtime, nil
		}
	}
	return RuntimeNPX, nil
}

var envVarPattern = regexp.MustCompile(`\b([A-Z][A-Z0-9_]{2,})\b`)

// extractEnvPrompts scans the README for likely environment variable names.
func (s *GitHubSource) extractEnvPrompts(ctx context.Context, repoPath string) ([]EnvPrompt, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/readme", repoPath)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.raw+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var prompts []EnvPrompt
	for _, match := range envVarPattern.FindAllString(string(body), -1) {
		if seen[match] {
			continue
		}
		// Only include names that look like env vars (contain _ and not all-word common terms)
		if !strings.Contains(match, "_") {
			continue
		}
		lower := strings.ToLower(match)
		isSecret := strings.Contains(lower, "token") || strings.Contains(lower, "key") ||
			strings.Contains(lower, "secret") || strings.Contains(lower, "pass") ||
			strings.Contains(lower, "api")
		seen[match] = true
		prompts = append(prompts, EnvPrompt{
			Key:      match,
			Required: false,
			Secret:   isSecret,
		})
		if len(prompts) >= 10 {
			break
		}
	}
	return prompts, nil
}
