package query

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	tool "github.com/koreaf16/argus/internal/tools"
)

type webEvidencePolicy struct {
	Enabled                     bool
	ResearchMode                bool
	MinSearches                 int
	MinFetches                  int
	MaxForcedRetries            int
	PreferOfficialURL           bool
	RequireDistinctFetchDomains bool
	PreferredDomains            []string
}

type webEvidenceState struct {
	SearchSeen            bool
	SearchAttempts        int
	SearchSuccesses       int
	NoResultSearches      int
	FetchSuccesses        int
	FetchHosts            map[string]bool
	ForcedRetries         int
	BufferedAssistantText string
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
		Enabled:                     false,
		ResearchMode:                false,
		MinSearches:                 1,
		MinFetches:                  1,
		MaxForcedRetries:            2,
		PreferOfficialURL:           true,
		RequireDistinctFetchDomains: false,
		PreferredDomains:            nil,
	}
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
	return policy.Enabled && (st.SearchSuccesses < policy.MinSearches || st.successfulFetches(policy) < policy.MinFetches)
}

func (st webEvidenceState) successfulFetches(policy webEvidencePolicy) int {
	if policy.RequireDistinctFetchDomains && len(st.FetchHosts) > 0 {
		return len(st.FetchHosts)
	}
	return st.FetchSuccesses
}

func buildWebEvidenceFollowUpPrompt(policy webEvidencePolicy, st webEvidenceState) string {
	var sb strings.Builder
	sb.WriteString("Do not finalize your answer yet.\n")
	sb.WriteString("Resume directly. Do not apologize, recap, or explain why you stopped.\n")
	if st.SearchSuccesses < policy.MinSearches {
		if len(policy.PreferredDomains) > 0 {
			sb.WriteString(fmt.Sprintf("1) Call %d web_search queries in the same response using allowed_domains=%v where useful, and include the current year when the request is time-sensitive.\n", max(1, policy.MinSearches-st.SearchSuccesses), policy.PreferredDomains))
			sb.WriteString("2) Also run one broader web_search without allowed_domains if the site-filtered results are weak or empty.\n")
		} else {
			sb.WriteString(fmt.Sprintf("1) Call %d-%d complementary web_search queries simultaneously in this response.\n", max(2, policy.MinSearches-st.SearchSuccesses), max(2, policy.MinSearches-st.SearchSuccesses+1)))
			sb.WriteString("2) Use different angles: official/vendor docs, practical checklist/runbook, and community/operator experience when relevant.\n")
		}
		if st.NoResultSearches > 0 {
			sb.WriteString("3) A previous search was weak or empty. Broaden or rephrase the query before concluding there are no results.\n")
		}
	} else {
		needed := max(1, policy.MinFetches-st.successfulFetches(policy))
		sb.WriteString(fmt.Sprintf("1) Select the best %d-%d URL(s) from the accumulated web_search results and call webfetch for them in the same response.\n", needed, max(needed, 2)))
		if len(policy.PreferredDomains) > 0 {
			sb.WriteString(fmt.Sprintf("2) Prefer these site hints first: %s\n", strings.Join(policy.PreferredDomains, ", ")))
		} else {
			sb.WriteString("2) Prefer official/primary sources first, then high-signal practical pages with concrete commands or procedures.\n")
		}
		if policy.RequireDistinctFetchDomains {
			sb.WriteString("3) Use distinct source domains unless the user explicitly asked for one site.\n")
		}
	}
	sb.WriteString(fmt.Sprintf("Preferred official domains: %s\n", strings.Join(preferredOfficialDomains, ", ")))
	sb.WriteString("For each webfetch, extract publication/update date, key facts, commands/procedures, and any caveats.\n")
	if policy.ResearchMode {
		sb.WriteString("After enough sources are fetched, compare them and synthesize the answer from the strongest evidence.\n")
	}
	sb.WriteString("Then provide the final answer with absolute dates when relevant and a Sources section.\n")
	sb.WriteString("If webfetch cannot be completed due to permission denial, explicitly say verification is incomplete.")
	return sb.String()
}

func buildWebEvidenceFailureMessage() string {
	return "Web verification was required but no successful webfetch was completed. I am not providing a snippet-only summary. Please allow or provide a fetchable source URL and retry."
}

func (st *webEvidenceState) ObserveToolCall(toolName string, result string, isErr bool) {
	switch tool.CanonicalName(toolName) {
	case "web_search":
		if isErr {
			return
		}
		st.SearchAttempts++
		st.SearchSeen = true
		if webSearchResultLooksUseful(result) {
			st.SearchSuccesses++
		} else {
			st.NoResultSearches++
		}
		return
	}
	if tool.CanonicalName(toolName) == "webfetch" && !isErr && webFetchResultLooksSuccessful(result) {
		st.FetchSuccesses++
		host := webFetchResultHost(result)
		if host == "" {
			host = fmt.Sprintf("__unknown_%d", st.FetchSuccesses)
		}
		if st.FetchHosts == nil {
			st.FetchHosts = make(map[string]bool)
		}
		st.FetchHosts[host] = true
	}
}

func webSearchResultLooksUseful(result string) bool {
	trimmed := strings.TrimSpace(result)
	if trimmed == "" {
		return false
	}
	lowered := strings.ToLower(trimmed)
	if strings.Contains(lowered, "no results found") {
		return false
	}
	return strings.Contains(lowered, `"url"`) || strings.Contains(lowered, "found ")
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

func webFetchResultHost(result string) string {
	var payload struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(result)), &payload); err != nil {
		return ""
	}
	u, err := url.Parse(strings.TrimSpace(payload.URL))
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))
}

