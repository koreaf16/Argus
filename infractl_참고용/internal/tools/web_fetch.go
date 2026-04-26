package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/web"
)

const (
	webFetchRedirectLimit = 5
	webFetchProbeTimeout  = 15 * time.Second
	webFetchUserAgent     = "infractl/1.0 (web_fetch redirect probe)"
	maxURLLength          = 2083
	shortContentBytes     = 256
)

type webFetchRedirectInfo struct {
	OriginalURL string
	RedirectURL string
	StatusCode  int
}

// WebFetchTool fetches one URL and applies prompt extraction.
type WebFetchTool struct {
	Fetcher     *web.Fetcher
	LLMClient   llm.Client
	LLMRegistry *llm.Registry
}

func (t *WebFetchTool) Name() string      { return "web_fetch" }
func (t *WebFetchTool) IsReadOnly() bool  { return true }
func (t *WebFetchTool) IsEnabled() bool   { return true }
func (t *WebFetchTool) IsLocalOnly() bool { return true }

func (t *WebFetchTool) IsConcurrencySafe(_ map[string]interface{}) bool {
	return true
}

func (t *WebFetchTool) Description() string {
	return "Fetch one URL, convert to Markdown, and extract requested details with the prompt."
}

func (t *WebFetchTool) ModelPrompt() string {
	return "Use web_fetch for deeper verification on a specific URL. Provide both url and prompt."
}

func (t *WebFetchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The URL to fetch content from.",
			},
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "The extraction prompt to apply to the fetched page content.",
			},
		},
		"required": []string{"url", "prompt"},
	}
}

func (t *WebFetchTool) Execute(ctx context.Context, args map[string]interface{}, _ executor.Executor) (ToolOutcome, error) {
	if _, hasLegacy := args["urls"]; hasLegacy {
		msg := "Error: web_fetch accepts a single url only. Use multiple web_fetch calls in parallel for multiple URLs."
		return ToolOutcome{
			Content:      msg,
			Success:      false,
			ExitCode:     2,
			ErrorMessage: msg,
			MetadataJSON: buildFetchMetadata("", false, 0, true, ""),
		}, nil
	}

	rawURL, err := argString(args, "url", true)
	if err != nil {
		return ToolOutcome{}, err
	}
	prompt, err := argString(args, "prompt", true)
	if err != nil {
		return ToolOutcome{}, err
	}

	normalizedURL, err := normalizeAndValidateWebFetchURL(rawURL)
	if err != nil {
		msg := fmt.Sprintf("Error: %s", err)
		return ToolOutcome{
			Content:      msg,
			Success:      false,
			ExitCode:     1,
			ErrorMessage: err.Error(),
			MetadataJSON: buildFetchMetadata("", false, 0, true, ""),
		}, nil
	}

	EmitOutput(ctx, fmt.Sprintf("query_update url=%s", normalizedURL))
	finalURL, redirectInfo, err := resolvePermittedRedirects(ctx, normalizedURL)
	if err != nil {
		msg := fmt.Sprintf("Fetch failed: %v", err)
		return ToolOutcome{
			Content:      msg,
			Success:      false,
			ExitCode:     1,
			ErrorMessage: err.Error(),
			MetadataJSON: buildFetchMetadata(normalizedURL, false, 0, true, ""),
		}, nil
	}
	if redirectInfo != nil {
		return ToolOutcome{
			Content:      renderRedirectMessage(*redirectInfo, prompt),
			Success:      true,
			MetadataJSON: buildFetchMetadata(redirectInfo.OriginalURL, true, 0, true, redirectInfo.RedirectURL),
		}, nil
	}

	fetcher := t.Fetcher
	if fetcher == nil {
		fetcher = web.NewFetcher(16, 0)
	}
	markdown, err := fetcher.Fetch(ctx, finalURL)
	if err != nil {
		msg := fmt.Sprintf("Fetch failed: %v", err)
		return ToolOutcome{
			Content:      msg,
			Success:      false,
			ExitCode:     1,
			ErrorMessage: err.Error(),
			MetadataJSON: buildFetchMetadata(finalURL, false, 0, true, ""),
		}, nil
	}
	EmitOutput(ctx, "search_results_received count=1")

	client := t.resolveFastClient()
	if client == nil {
		content := fmt.Sprintf(
			"Content from %s:\n\n%s\n\nWarning: prompt processing unavailable; returning raw page content.",
			finalURL,
			markdown,
		)
		followUp := visibleMarkdownBytes(markdown) < shortContentBytes
		return ToolOutcome{
			Content:      content,
			Success:      true,
			MetadataJSON: buildFetchMetadata(finalURL, false, len(markdown), followUp, ""),
		}, nil
	}

	processed, err := t.applyPrompt(ctx, client, finalURL, markdown, prompt)
	if err != nil {
		content := fmt.Sprintf(
			"Content from %s:\n\n%s\n\nWarning: prompt processing failed (%v); returning raw page content.",
			finalURL,
			markdown,
			err,
		)
		return ToolOutcome{
			Content:      content,
			Success:      true,
			MetadataJSON: buildFetchMetadata(finalURL, false, len(markdown), true, ""),
		}, nil
	}

	followUp := visibleMarkdownBytes(markdown) < shortContentBytes
	return ToolOutcome{
		Content:      fmt.Sprintf("Content from %s:\n\n%s", finalURL, strings.TrimSpace(processed)),
		Success:      true,
		MetadataJSON: buildFetchMetadata(finalURL, false, len(markdown), followUp, ""),
	}, nil
}

func buildFetchMetadata(finalURL string, redirectBlocked bool, contentBytes int, followUp bool, followUpURL string) string {
	b, _ := json.Marshal(map[string]any{
		"kind":               "web_fetch",
		"final_url":          finalURL,
		"redirect_blocked":   redirectBlocked,
		"content_bytes":      contentBytes,
		"follow_up_required": followUp,
		"follow_up_url":      followUpURL,
	})
	return string(b)
}

func visibleMarkdownBytes(markdown string) int {
	replacer := strings.NewReplacer(
		"#", " ",
		"*", " ",
		"_", " ",
		"`", " ",
		"[", " ",
		"]", " ",
		"(", " ",
		")", " ",
		"-", " ",
		">", " ",
	)
	cleaned := strings.Join(strings.Fields(replacer.Replace(markdown)), " ")
	return len(cleaned)
}

func normalizeAndValidateWebFetchURL(raw string) (string, error) {
	if len(strings.TrimSpace(raw)) == 0 {
		return "", fmt.Errorf("no URL provided")
	}
	if len(raw) > maxURLLength {
		return "", fmt.Errorf("invalid URL")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid URL")
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid URL")
	}
	if parsed.User != nil && parsed.User.Username() != "" {
		return "", fmt.Errorf("invalid URL")
	}
	host := parsed.Hostname()
	if host == "" {
		return "", fmt.Errorf("invalid URL")
	}
	isLocal := isLoopbackOrLocalHost(host)
	parts := strings.Split(host, ".")
	if !isLocal && len(parts) < 2 {
		return "", fmt.Errorf("invalid URL")
	}
	if parsed.Scheme == "http" && !isLocal {
		parsed.Scheme = "https"
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLocal) {
		return "", fmt.Errorf("invalid URL")
	}
	return parsed.String(), nil
}

func isLoopbackOrLocalHost(host string) bool {
	normalized := strings.TrimSpace(strings.ToLower(host))
	if normalized == "localhost" {
		return true
	}
	ip := net.ParseIP(normalized)
	return ip != nil && ip.IsLoopback()
}

func resolvePermittedRedirects(ctx context.Context, startURL string) (string, *webFetchRedirectInfo, error) {
	client := &http.Client{
		Timeout: webFetchProbeTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	current := startURL
	original := startURL
	for i := 0; i < webFetchRedirectLimit; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, current, nil)
		if err != nil {
			return "", nil, err
		}
		req.Header.Set("User-Agent", webFetchUserAgent)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

		resp, err := client.Do(req)
		if err != nil {
			return "", nil, err
		}
		_ = resp.Body.Close()

		if !isRedirectStatus(resp.StatusCode) {
			return current, nil, nil
		}

		location := strings.TrimSpace(resp.Header.Get("Location"))
		if location == "" {
			return "", nil, fmt.Errorf("redirect missing Location header")
		}
		nextURL, err := resolveRedirectURL(current, location)
		if err != nil {
			return "", nil, err
		}

		if !isPermittedRedirect(current, nextURL) {
			return "", &webFetchRedirectInfo{
				OriginalURL: original,
				RedirectURL: nextURL,
				StatusCode:  resp.StatusCode,
			}, nil
		}
		current = nextURL
	}
	return "", nil, fmt.Errorf("too many redirects")
}

func isRedirectStatus(code int) bool {
	return code == http.StatusMovedPermanently ||
		code == http.StatusFound ||
		code == http.StatusTemporaryRedirect ||
		code == http.StatusPermanentRedirect
}

func resolveRedirectURL(baseURL, location string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(location)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(ref).String(), nil
}

func isPermittedRedirect(originalURL, redirectURL string) bool {
	orig, err := url.Parse(originalURL)
	if err != nil {
		return false
	}
	next, err := url.Parse(redirectURL)
	if err != nil {
		return false
	}
	if next.Scheme != orig.Scheme {
		return false
	}
	if next.Port() != orig.Port() {
		return false
	}
	if next.User != nil && next.User.Username() != "" {
		return false
	}

	stripWww := func(host string) string { return strings.TrimPrefix(strings.ToLower(host), "www.") }
	return stripWww(orig.Hostname()) == stripWww(next.Hostname())
}

func renderRedirectMessage(info webFetchRedirectInfo, prompt string) string {
	statusText := http.StatusText(info.StatusCode)
	if statusText == "" {
		statusText = "Redirect"
	}
	return fmt.Sprintf(
		"REDIRECT DETECTED: The URL redirects to a different host.\n\nOriginal URL: %s\nRedirect URL: %s\nStatus: %d %s\n\nTo complete your request, I need to fetch content from the redirected URL. Please use web_fetch again with these parameters:\n- url: %q\n- prompt: %q",
		info.OriginalURL,
		info.RedirectURL,
		info.StatusCode,
		statusText,
		info.RedirectURL,
		prompt,
	)
}

func (t *WebFetchTool) resolveFastClient() llm.Client {
	if t.LLMRegistry != nil {
		if c, _, err := t.LLMRegistry.Resolve(llm.TierFast); err == nil {
			return c
		}
	}
	return t.LLMClient
}

func (t *WebFetchTool) applyPrompt(ctx context.Context, client llm.Client, rawURL, markdown, prompt string) (string, error) {
	userMsg := llm.Message{
		Role: llm.RoleUser,
		Content: fmt.Sprintf(
			"URL: %s\n\nPage content:\n%s\n\n---\nQuestion: %s",
			rawURL,
			markdown,
			prompt,
		),
	}
	resp, err := client.Chat(ctx, []llm.Message{userMsg}, nil, nil)
	if err != nil {
		return "", fmt.Errorf("llm apply prompt: %w", err)
	}
	return strings.TrimSpace(resp.Content), nil
}
