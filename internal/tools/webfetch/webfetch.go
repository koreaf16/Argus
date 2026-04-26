package webfetch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/webconfig"
	"github.com/koreaf16/argus/internal/types"
	"github.com/koreaf16/argus/internal/utils/permissions"
)

const (
	toolName                = "webfetch"
	defaultEventBufferSize  = 2
	maxPromptMarkdownLength = 100_000
)

var builtInTrustedHosts = []string{
	"ai.google.dev",
	"deepmind.google",
	"blog.google",
	"developers.googleblog.com",
	"developer.android.com",
	"huggingface.co",
	"cloud.google.com",
}

type WebFetchTool struct {
	client       *http.Client
	crawlBaseURL string
}

type fetchInput struct {
	URL    string `json:"url"`
	Prompt string `json:"prompt"`
}

type fetchOutput struct {
	Bytes      int    `json:"bytes"`
	Code       int    `json:"code"`
	CodeText   string `json:"codeText"`
	Result     string `json:"result"`
	DurationMS int64  `json:"durationMs"`
	URL        string `json:"url"`
}

type crawlFetch struct {
	Markdown string
	FinalURL string
	Code     int
	CodeText string
}

func NewWebFetchTool() *WebFetchTool {
	cfg := webconfig.Load()
	timeout := time.Duration(cfg.Crawl4AITimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Duration(webconfig.DefaultCrawl4AITimeout) * time.Millisecond
	}
	return &WebFetchTool{
		client:       &http.Client{Timeout: timeout},
		crawlBaseURL: strings.TrimRight(strings.TrimSpace(cfg.Crawl4AIBase), "/"),
	}
}

func (t *WebFetchTool) Name() string {
	return toolName
}

func (t *WebFetchTool) Description(ctx tool.Context) string {
	_ = ctx
	return "Fetch and extract web page content via Crawl4AI, then apply a prompt to the extracted markdown. For time-sensitive or latest/news queries, extract publication/update date, key facts, what changed, and include the canonical source URL."
}

func (t *WebFetchTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "The URL to fetch",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "What information to extract from the fetched content",
			},
		},
		"required":             []string{"url", "prompt"},
		"additionalProperties": false,
	}
}

func (t *WebFetchTool) IsReadOnly() bool {
	return true
}

func (t *WebFetchTool) MaxResultSizeChars() int {
	return 30000
}

func (t *WebFetchTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	req, err := parseInput(input)
	if err != nil {
		return nil, err
	}

	events := make(chan tool.ToolEvent, defaultEventBufferSize)
	go func() {
		defer close(events)

		callCtx := ctx.Context
		if callCtx == nil {
			callCtx = context.Background()
		}

		started := time.Now()
		fetched, err := t.fetchMarkdown(callCtx, req.URL)
		if err != nil {
			events <- tool.NewErrorEvent(err)
			events <- tool.NewDoneEvent()
			return
		}

		// Do not silently follow cross-host redirects. Ask model to re-request.
		if redirectURL, ok := crossHostRedirect(req.URL, fetched.FinalURL); ok {
			msg := fmt.Sprintf(
				"REDIRECT DETECTED: The URL redirects to a different host.\n\nOriginal URL: %s\nRedirect URL: %s\n\nUse webfetch again with this url and the same prompt.",
				req.URL,
				redirectURL,
			)
			out := fetchOutput{
				Bytes:      len([]byte(msg)),
				Code:       http.StatusFound,
				CodeText:   http.StatusText(http.StatusFound),
				Result:     msg,
				DurationMS: time.Since(started).Milliseconds(),
				URL:        req.URL,
			}
			emitJSONOutput(events, out)
			return
		}

		resultText, subErr := applyPrompt(ctx, req.Prompt, fetched.FinalURL, fetched.Markdown)
		if subErr != nil {
			resultText = fetched.Markdown
		}

		out := fetchOutput{
			Bytes:      len([]byte(fetched.Markdown)),
			Code:       fetched.Code,
			CodeText:   fetched.CodeText,
			Result:     resultText,
			DurationMS: time.Since(started).Milliseconds(),
			URL:        req.URL,
		}
		emitJSONOutput(events, out)
	}()

	return events, nil
}

func emitJSONOutput(events chan<- tool.ToolEvent, out fetchOutput) {
	encoded, err := json.Marshal(out)
	if err != nil {
		events <- tool.NewErrorEvent(err)
		events <- tool.NewDoneEvent()
		return
	}
	events <- tool.NewOutputEvent(string(encoded))
	events <- tool.NewDoneEvent()
}

func parseInput(input json.RawMessage) (fetchInput, error) {
	var req fetchInput
	if err := json.Unmarshal(input, &req); err != nil {
		return fetchInput{}, fmt.Errorf("invalid webfetch input: %w", err)
	}
	req.URL = strings.TrimSpace(req.URL)
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.URL == "" {
		return fetchInput{}, fmt.Errorf("url is required")
	}
	if req.Prompt == "" {
		return fetchInput{}, fmt.Errorf("prompt is required")
	}
	if _, err := url.ParseRequestURI(req.URL); err != nil {
		return fetchInput{}, fmt.Errorf("invalid url: %w", err)
	}
	return req, nil
}

func (t *WebFetchTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	req, err := parseInput(input)
	if err != nil {
		return tool.PermissionResult{
			Behavior: types.BehaviorDeny,
			Message:  err.Error(),
		}, nil
	}
	host := normalizedHost(req.URL)
	if host == "" {
		return tool.PermissionResult{
			Behavior: types.BehaviorDeny,
			Message:  "invalid url host",
		}, nil
	}

	permCtx := permissions.NewDefaultPermissionContext()
	if ctx.State != nil {
		permCtx.Mode = ctx.State.GetPermissionMode()
	}

	if rule := findMatchingRule(permissions.GetDenyRules(permCtx), host); rule != nil {
		return tool.PermissionResult{
			Behavior: types.BehaviorDeny,
			Message:  fmt.Sprintf("webfetch denied for host: %s", host),
			DecisionReason: &types.PermissionDecisionReason{
				Type: types.DecisionReasonRule,
				Rule: rule,
			},
		}, nil
	}
	if rule := findMatchingRule(permissions.GetAskRules(permCtx), host); rule != nil {
		return tool.PermissionResult{
			Behavior: types.BehaviorAsk,
			Message:  fmt.Sprintf("webfetch requires approval for host: %s", host),
			DecisionReason: &types.PermissionDecisionReason{
				Type: types.DecisionReasonRule,
				Rule: rule,
			},
		}, nil
	}
	if rule := findMatchingRule(permissions.GetAllowRules(permCtx), host); rule != nil {
		return tool.PermissionResult{
			Behavior: types.BehaviorAllow,
			DecisionReason: &types.PermissionDecisionReason{
				Type: types.DecisionReasonRule,
				Rule: rule,
			},
		}, nil
	}
	if isBuiltInTrustedHost(host) {
		return tool.PermissionResult{
			Behavior: types.BehaviorAllow,
			DecisionReason: &types.PermissionDecisionReason{
				Type:   types.DecisionReasonOther,
				Reason: "built-in trusted host",
			},
		}, nil
	}

	switch permCtx.Mode {
	case types.PermissionModeBypassPermissions, types.PermissionModeDontAsk, types.PermissionModeAcceptEdits:
		return tool.PermissionResult{Behavior: types.BehaviorAllow}, nil
	default:
		// webfetch is read-only, allow by default unless explicit rules exist
		return tool.PermissionResult{Behavior: types.BehaviorAllow}, nil
	}
}

func findMatchingRule(rules []types.PermissionRule, host string) *types.PermissionRule {
	for _, rule := range rules {
		if !strings.EqualFold(strings.TrimSpace(rule.RuleValue.ToolName), toolName) {
			continue
		}
		content := strings.TrimSpace(rule.RuleValue.RuleContent)
		if content == "" {
			matched := rule
			return &matched
		}
		pattern, ok := extractDomainPattern(content)
		if !ok {
			continue
		}
		if domainMatches(host, pattern) {
			matched := rule
			return &matched
		}
	}
	return nil
}

func extractDomainPattern(content string) (string, bool) {
	v := strings.TrimSpace(content)
	if strings.HasPrefix(strings.ToLower(v), "domain:") {
		p := strings.TrimSpace(v[len("domain:"):])
		if p == "" {
			return "", false
		}
		return p, true
	}
	return v, true
}

func domainMatches(host, pattern string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if host == "" || pattern == "" {
		return false
	}
	if strings.HasPrefix(pattern, "*.") {
		base := strings.TrimPrefix(pattern, "*.")
		return host == base || strings.HasSuffix(host, "."+base)
	}
	if strings.Contains(pattern, "*") {
		return permissions.MatchWildcardPattern(pattern, host, true)
	}
	return host == pattern || strings.HasSuffix(host, "."+pattern)
}

func isBuiltInTrustedHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	for _, pattern := range builtInTrustedHosts {
		if domainMatches(host, pattern) {
			return true
		}
	}
	return false
}

func (t *WebFetchTool) fetchMarkdown(ctx context.Context, rawURL string) (crawlFetch, error) {
	if err := validateURLForSSRF(rawURL); err != nil {
		return crawlFetch{}, fmt.Errorf("webfetch blocked: %w", err)
	}
	if strings.TrimSpace(t.crawlBaseURL) == "" {
		return crawlFetch{}, fmt.Errorf("crawl4ai base url is empty")
	}
	endpoint := t.crawlBaseURL + "/crawl"

	reqBody := map[string]any{
		"urls": []string{rawURL},
		"browser_config": map[string]any{
			"type": "BrowserConfig",
			"params": map[string]any{
				"headless": true,
			},
		},
		"crawler_config": map[string]any{
			"type": "CrawlerRunConfig",
			"params": map[string]any{
				"stream":     false,
				"cache_mode": "bypass",
			},
		},
	}
	encodedBody, err := json.Marshal(reqBody)
	if err != nil {
		return crawlFetch{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encodedBody))
	if err != nil {
		return crawlFetch{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return crawlFetch{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return crawlFetch{}, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(respBody))
		if msg == "" {
			msg = "empty response body"
		}
		return crawlFetch{}, fmt.Errorf("crawl4ai request failed: status=%d body=%s", resp.StatusCode, msg)
	}

	var payload map[string]any
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return crawlFetch{}, fmt.Errorf("decode crawl4ai response: %w", err)
	}

	if ok, hasSuccess := payload["success"].(bool); hasSuccess && !ok {
		return crawlFetch{}, fmt.Errorf("crawl4ai returned success=false")
	}

	result := extractCrawlResult(payload)
	if result == nil {
		return crawlFetch{}, fmt.Errorf("crawl4ai response missing result payload")
	}
	markdown := extractMarkdown(result)
	if strings.TrimSpace(markdown) == "" {
		return crawlFetch{}, fmt.Errorf("crawl4ai response missing markdown content")
	}

	finalURL := extractURL(result)
	if finalURL == "" {
		finalURL = extractURL(payload)
	}
	if finalURL == "" {
		finalURL = rawURL
	}

	return crawlFetch{
		Markdown: markdown,
		FinalURL: finalURL,
		Code:     resp.StatusCode,
		CodeText: http.StatusText(resp.StatusCode),
	}, nil
}

func extractCrawlResult(root map[string]any) map[string]any {
	if root == nil {
		return nil
	}
	if _, ok := root["markdown"]; ok {
		return root
	}
	if v, ok := root["result"].(map[string]any); ok {
		return v
	}
	if arr, ok := root["results"].([]any); ok {
		for _, item := range arr {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if succ, ok := m["success"].(bool); ok && !succ {
				continue
			}
			return m
		}
	}
	if v, ok := root["data"].(map[string]any); ok {
		return extractCrawlResult(v)
	}
	return nil
}

func extractMarkdown(result map[string]any) string {
	if result == nil {
		return ""
	}
	if markdownValue, ok := result["markdown"]; ok {
		switch v := markdownValue.(type) {
		case string:
			return strings.TrimSpace(v)
		case map[string]any:
			if s, ok := v["fit_markdown"].(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
			if s, ok := v["raw_markdown"].(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
			if s, ok := v["markdown"].(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	if s, ok := result["fit_markdown"].(string); ok && strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	if s, ok := result["raw_markdown"].(string); ok && strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	return ""
}

func extractURL(m map[string]any) string {
	if m == nil {
		return ""
	}
	for _, key := range []string{"final_url", "finalUrl", "url", "source_url"} {
		if s, ok := m[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func crossHostRedirect(originalURL, finalURL string) (string, bool) {
	if strings.TrimSpace(finalURL) == "" {
		return "", false
	}
	originalHost := canonicalHost(originalURL)
	finalHost := canonicalHost(finalURL)
	if originalHost == "" || finalHost == "" {
		return "", false
	}
	if originalHost == finalHost {
		return "", false
	}
	return finalURL, true
}

func canonicalHost(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	return strings.TrimPrefix(host, "www.")
}

func normalizedHost(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parsed.Hostname()))
}

func applyPrompt(ctx tool.Context, prompt, sourceURL, markdown string) (string, error) {
	if strings.TrimSpace(markdown) == "" {
		return "", fmt.Errorf("empty markdown content")
	}
	if ctx.ExecuteSubQuery == nil {
		return markdown, nil
	}

	callCtx := ctx.Context
	if callCtx == nil {
		callCtx = context.Background()
	}

	content := markdown
	if len(content) > maxPromptMarkdownLength {
		content = content[:maxPromptMarkdownLength] + "\n\n[Truncated due to length]"
	}

	systemPrompt := "You are a web content extraction assistant. Use only the provided markdown content."
	userPrompt := fmt.Sprintf(
		"URL: %s\n\nExtraction Prompt:\n%s\n\nMarkdown Content:\n%s",
		sourceURL,
		prompt,
		content,
	)
	response, err := ctx.ExecuteSubQuery(callCtx, systemPrompt, userPrompt)
	if err != nil {
		return "", err
	}
	response = strings.TrimSpace(response)
	if response == "" {
		return markdown, nil
	}
	return response, nil
}
