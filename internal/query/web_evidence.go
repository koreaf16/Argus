package query

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/koreaf16/argus/internal/services/llm"
)

type subQueryExecutor func(ctx context.Context, systemPrompt string, userPrompt string) (string, error)

type webEvidencePolicy struct {
	Enabled           bool
	MinFetches        int
	MaxForcedRetries  int
	PreferOfficialURL bool
	PreferredDomains  []string
}

type webEvidenceState struct {
	SearchSeen            bool
	FetchSuccesses        int
	ForcedRetries         int
	BufferedAssistantText string
}

var strongWebEnableTerms = []string{
	"today", "latest", "recent", "news", "current", "this week", "breaking", "release", "changelog",
	"official", "documentation", "model card", "repository", "repo", "version", "up to date", "up-to-date",
	"오늘", "최신", "최근", "현재", "이번 주", "뉴스", "방금", "실시간", "공식", "버전", "릴리스",
	"모델", "모델카드", "레포", "저장소", "찾아봐", "검색해", "확인해",
	"hugging face", "huggingface", "huggingface.co", "github", "github.com", "docker", "docker hub", "hub.docker.com",
	"pypi", "pypi.org", "pip", "npm", "npmjs", "npmjs.com", "registry.npmjs.org", "maven", "maven central", "search.maven.org",
	"nuget", "nuget.org", "api.nuget.org", "artifacthub", "artifact hub", "artifacthub.io",
}

var relativeDateTerms = []string{
	"today", "yesterday", "tomorrow", "this week", "this month", "this year",
	"이번 주", "이번달", "이번 달", "올해", "어제", "오늘", "내일",
}

var strongWebDisableTerms = []string{
	"refactor", "refactoring", "unit test", "fix bug", "debug this code", "implement", "write code",
	"explain this code", "review this code", "translate this", "summarize this text",
	"refactor this", "in this repository", "local file",
	"리팩터", "테스트 코드", "버그 수정", "코드 수정", "코드 리뷰", "이 코드", "번역", "요약",
}

var externalFactHintTerms = []string{
	"release", "changelog", "version", "documentation", "docs", "pricing", "policy", "status", "outage",
	"announcement", "ceo", "president", "market", "stock", "exchange rate", "schedule", "roadmap",
	"model", "model card", "repository", "repo", "artifact", "package", "container image",
	"출시", "릴리스", "변경사항", "버전", "문서", "가격", "정책", "현황", "공지", "일정", "모델", "레포",
}

var externalLookupActionTerms = []string{
	"search", "look up", "lookup", "find", "check", "verify", "browse", "from",
	"찾아", "검색", "조회", "확인", "검증", "알아봐", "에서",
}

var preferredOfficialDomains = []string{
	"ai.google.dev",
	"deepmind.google",
	"blog.google",
	"developers.googleblog.com",
	"developer.android.com",
	"huggingface.co",
	"github.com",
	"hub.docker.com",
	"docs.docker.com",
	"pypi.org",
	"npmjs.com",
	"registry.npmjs.org",
	"search.maven.org",
	"central.sonatype.org",
	"nuget.org",
	"api.nuget.org",
	"artifacthub.io",
	"cloud.google.com",
}

func defaultWebEvidencePolicy() webEvidencePolicy {
	return webEvidencePolicy{
		Enabled:           false,
		MinFetches:        1,
		MaxForcedRetries:  2,
		PreferOfficialURL: true,
		PreferredDomains:  nil,
	}
}

func classifyWebEvidencePolicy(ctx context.Context, executeSubQuery subQueryExecutor, userText string, recentConversation string) webEvidencePolicy {
	policy := defaultWebEvidencePolicy()
	normalized := normalizeIntentText(userText)
	if normalized == "" {
		return policy
	}
	policy.PreferredDomains = detectPreferredDomains(normalized)

	if hasAnyTerm(normalized, strongWebEnableTerms) || hasAnyTerm(normalized, relativeDateTerms) {
		policy.Enabled = true
		return policy
	}
	if hasAnyTerm(normalized, strongWebDisableTerms) {
		return policy
	}
	if isLikelyExternalKnowledgeRequest(normalized, policy.PreferredDomains) {
		policy.Enabled = true
		return policy
	}
	if !hasAnyTerm(normalized, externalFactHintTerms) {
		return policy
	}

	enabled, err := classifyFreshWebIntentByLLM(ctx, executeSubQuery, userText, recentConversation)
	if err != nil {
		// Conservative fallback: if we reached LLM fallback and parsing failed, force verification.
		policy.Enabled = true
		return policy
	}
	policy.Enabled = enabled
	return policy
}

func classifyFreshWebIntentByLLM(ctx context.Context, executeSubQuery subQueryExecutor, userText string, recentConversation string) (bool, error) {
	if executeSubQuery == nil {
		return false, fmt.Errorf("subquery executor is nil")
	}

	systemPrompt := "You are an intent classifier. Decide whether a user query requires fresh web evidence. Output exactly one token: ENABLE or SKIP."
	userPrompt := fmt.Sprintf(
		"Today: %s\n\nCurrent user query:\n%s\n\nRecent conversation context:\n%s\n\nDecision rule:\n- ENABLE: accurate answer requires up-to-date web verification.\n- SKIP: request can be answered without fresh web verification.\n\nOutput exactly ENABLE or SKIP.",
		time.Now().Format("2006-01-02"),
		strings.TrimSpace(userText),
		strings.TrimSpace(recentConversation),
	)

	resp, err := executeSubQuery(ctx, systemPrompt, userPrompt)
	if err != nil {
		return false, err
	}
	decision, parseErr := parseWebIntentDecision(resp)
	if parseErr != nil {
		return false, parseErr
	}
	return decision, nil
}

func parseWebIntentDecision(raw string) (bool, error) {
	tokens := strings.Fields(strings.ToUpper(strings.TrimSpace(raw)))
	for _, token := range tokens {
		clean := strings.Trim(token, "\"'`.,:;!?()[]{}")
		switch clean {
		case "ENABLE":
			return true, nil
		case "SKIP":
			return false, nil
		}
	}
	return false, fmt.Errorf("unable to parse web intent decision: %q", raw)
}

func normalizeIntentText(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func detectPreferredDomains(text string) []string {
	domains := make([]string, 0, 3)
	appendIfMissing := func(domain string) {
		for _, existing := range domains {
			if existing == domain {
				return
			}
		}
		domains = append(domains, domain)
	}

	if hasAnyTerm(text, []string{"huggingface.co", "hugging face", "huggingface", "허깅페이스"}) {
		appendIfMissing("huggingface.co")
	}
	if hasAnyTerm(text, []string{"github.com", "github", "깃허브"}) {
		appendIfMissing("github.com")
	}
	if hasAnyTerm(text, []string{"hub.docker.com", "docker hub", "dockerhub", "도커"}) {
		appendIfMissing("hub.docker.com")
	}
	if hasAnyTerm(text, []string{"pypi.org", "pypi", "pip", "python package"}) {
		appendIfMissing("pypi.org")
	}
	if hasAnyTerm(text, []string{"npmjs.com", "registry.npmjs.org", "npm", "node package"}) {
		appendIfMissing("npmjs.com")
	}
	if hasAnyTerm(text, []string{"search.maven.org", "central.sonatype.org", "maven", "maven central"}) {
		appendIfMissing("search.maven.org")
	}
	if hasAnyTerm(text, []string{"nuget.org", "api.nuget.org", "nuget", "dotnet package", ".net package"}) {
		appendIfMissing("nuget.org")
	}
	if hasAnyTerm(text, []string{"artifacthub.io", "artifacthub", "artifact hub", "helm chart"}) {
		appendIfMissing("artifacthub.io")
	}
	return domains
}

func isLikelyExternalKnowledgeRequest(text string, preferredDomains []string) bool {
	if len(preferredDomains) > 0 {
		return true
	}
	if hasAnyTerm(text, externalFactHintTerms) {
		return true
	}
	if hasAnyTerm(text, externalLookupActionTerms) && hasAnyTerm(text, []string{
		"release", "version", "model", "repo", "repository", "docs", "document",
		"최신", "버전", "모델", "레포", "문서",
	}) {
		return true
	}
	return false
}

func hasAnyTerm(text string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(text, strings.ToLower(term)) {
			return true
		}
	}
	return false
}

func shouldBufferAssistantText(policy webEvidencePolicy, st webEvidenceState) bool {
	return policy.Enabled && st.FetchSuccesses < policy.MinFetches
}

func shouldForceWebEvidenceContinuation(policy webEvidencePolicy, st webEvidenceState) bool {
	if !shouldBufferAssistantText(policy, st) {
		return false
	}
	return st.ForcedRetries < policy.MaxForcedRetries
}

func buildWebEvidenceFollowUpPrompt(policy webEvidencePolicy, st webEvidenceState) string {
	var sb strings.Builder
	sb.WriteString("Do not finalize your answer yet.\n")
	if !st.SearchSeen {
		if len(policy.PreferredDomains) > 0 {
			sb.WriteString(fmt.Sprintf("1) Call web_search first using allowed_domains=%v and include the current year in the query.\n", policy.PreferredDomains))
			sb.WriteString("2) If no strong results are found, run one broader web_search without allowed_domains.\n")
		} else {
			sb.WriteString("1) Call web_search first using broad terms and the current year.\n")
			sb.WriteString("2) Prefer official sources when available.\n")
		}
	} else {
		sb.WriteString("1) Select the best URL from the existing web_search results.\n")
		if len(policy.PreferredDomains) > 0 {
			sb.WriteString(fmt.Sprintf("2) Prefer these site hints first: %s\n", strings.Join(policy.PreferredDomains, ", ")))
		} else {
			sb.WriteString("2) Prefer official sources when available.\n")
		}
	}
	sb.WriteString(fmt.Sprintf("Preferred official domains: %s\n", strings.Join(preferredOfficialDomains, ", ")))
	sb.WriteString("3) Call webfetch on one selected URL.\n")
	sb.WriteString("4) Extract publication/update date, key facts, and what changed.\n")
	sb.WriteString("5) Then provide the final answer with absolute dates and a Sources section.\n")
	sb.WriteString("If webfetch cannot be completed due to permission denial, explicitly say verification is incomplete.")
	return sb.String()
}

func buildWebEvidenceFailureMessage() string {
	return "Web verification was required but no successful webfetch was completed. I am not providing a snippet-only summary. Please allow or provide a fetchable source URL and retry."
}

func (st *webEvidenceState) ObserveToolCall(toolName string, result string, isErr bool) {
	if strings.EqualFold(strings.TrimSpace(toolName), "web_search") && !isErr {
		st.SearchSeen = true
		return
	}
	if strings.EqualFold(strings.TrimSpace(toolName), "webfetch") && !isErr && webFetchResultLooksSuccessful(result) {
		st.FetchSuccesses++
	}
}

func webFetchResultLooksSuccessful(result string) bool {
	trimmed := strings.TrimSpace(result)
	if trimmed == "" {
		return false
	}
	lowered := strings.ToLower(trimmed)
	if strings.Contains(lowered, "redirect detected") {
		return false
	}
	if strings.Contains(lowered, "permission denied for webfetch") {
		return false
	}

	var payload struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return true
	}
	if payload.Code == 0 {
		return true
	}
	return payload.Code >= 200 && payload.Code < 300
}

func buildRecentConversationHint(messages []llm.Message, maxChars int) string {
	if maxChars <= 0 {
		maxChars = 800
	}
	collected := make([]string, 0, 6)
	used := 0

	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != llm.RoleUser && msg.Role != llm.RoleAssistant {
			continue
		}
		text := extractFirstTextBlock(msg)
		if text == "" {
			continue
		}
		role := "assistant"
		if msg.Role == llm.RoleUser {
			role = "user"
		}
		segment := role + ": " + text
		if used+len(segment) > maxChars {
			remain := maxChars - used
			if remain <= 20 {
				break
			}
			segment = segment[:remain]
		}
		collected = append(collected, segment)
		used += len(segment)
		if used >= maxChars || len(collected) >= 6 {
			break
		}
	}

	for i, j := 0, len(collected)-1; i < j; i, j = i+1, j-1 {
		collected[i], collected[j] = collected[j], collected[i]
	}
	return strings.Join(collected, "\n")
}

func extractFirstTextBlock(msg llm.Message) string {
	for _, block := range msg.Content {
		if block.Type != llm.ContentText {
			continue
		}
		text := strings.TrimSpace(block.Text)
		if text != "" {
			return text
		}
	}
	return ""
}
