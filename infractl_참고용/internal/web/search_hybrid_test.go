package web

import "testing"

func TestPickAdaptersByIntent(t *testing.T) {
	adapters := pickAdapters(
		"vllm-project/vllm latest release on github and huggingface model",
		SearchOptions{},
		SearchAdapterConfig{
			GitHub:      true,
			HuggingFace: true,
			DockerHub:   true,
			PyPI:        true,
			NPM:         true,
			OSV:         true,
		},
	)
	got := map[string]bool{}
	for _, a := range adapters {
		got[a.Name()] = true
	}
	if !got["github"] {
		t.Fatalf("expected github adapter")
	}
	if !got["huggingface"] {
		t.Fatalf("expected huggingface adapter")
	}
}

func TestPickAdaptersHonorsAllowedDomains(t *testing.T) {
	adapters := pickAdapters(
		"docker image nginx",
		SearchOptions{AllowedDomains: []string{"github.com"}},
		SearchAdapterConfig{
			GitHub:      true,
			HuggingFace: true,
			DockerHub:   true,
			PyPI:        true,
			NPM:         true,
			OSV:         true,
		},
	)
	if len(adapters) != 0 {
		t.Fatalf("expected no adapters for docker query when only github is allowed, got %d", len(adapters))
	}
}

func TestRankAndDedupePrefersOfficialExact(t *testing.T) {
	results := []SearchResult{
		{
			Title:      "vllm - random blog",
			URL:        "https://example.tistory.com/vllm",
			Snippet:    "blog post",
			Provider:   "",
			ExactMatch: false,
		},
		{
			Title:      "vllm-project/vllm · GitHub",
			URL:        "https://github.com/vllm-project/vllm",
			Snippet:    "official repo",
			Provider:   "github",
			ExactMatch: true,
		},
		{
			Title:      "vllm-project/vllm · GitHub duplicate",
			URL:        "https://github.com/vllm-project/vllm",
			Snippet:    "duplicate",
			Provider:   "github",
			ExactMatch: false,
		},
	}

	ranked := rankAndDedupe(results, "vllm github release", SearchOptions{})
	if len(ranked) < 2 {
		t.Fatalf("expected at least 2 ranked results, got %d", len(ranked))
	}
	if ranked[0].Provider != "github" {
		t.Fatalf("expected github result first, got provider=%q title=%q", ranked[0].Provider, ranked[0].Title)
	}
	if ranked[0].URL != "https://github.com/vllm-project/vllm" {
		t.Fatalf("unexpected first URL: %s", ranked[0].URL)
	}
}

func TestAppendDomainHints(t *testing.T) {
	got := appendDomainHints("terraform aws provider latest")
	if got == "terraform aws provider latest" {
		t.Fatalf("expected appended domain hint, got unchanged query")
	}
	if !containsFold(got, "site:registry.terraform.io") {
		t.Fatalf("expected terraform site hint in query: %q", got)
	}
}

func TestExtractVulnerabilityID(t *testing.T) {
	for query, want := range map[string]string{
		"CVE-2025-12345 details":             "CVE-2025-12345",
		"please check ghsa-aaaa-bbbb-cccc":   "GHSA-AAAA-BBBB-CCCC",
		"no vulnerability id in this string": "",
	} {
		if got := extractVulnerabilityID(query); got != want {
			t.Fatalf("extractVulnerabilityID(%q)=%q, want %q", query, got, want)
		}
	}
}

func TestNormalizeHuggingFaceSearchQuery(t *testing.T) {
	got := normalizeHuggingFaceSearchQuery("huggingface model vllm")
	if got != "vllm" {
		t.Fatalf("expected normalized query to be vllm, got %q", got)
	}
}

func TestNormalizeDockerHubSearchQuery(t *testing.T) {
	got := normalizeDockerHubSearchQuery("docker image nginx")
	if got != "nginx" {
		t.Fatalf("expected normalized docker query to be nginx, got %q", got)
	}
}

func TestEndpointHelpers(t *testing.T) {
	if got := endpointOrDefault(" https://api.example.com/ ", "https://fallback.example.com"); got != "https://api.example.com/" {
		t.Fatalf("unexpected endpointOrDefault result: %q", got)
	}
	if got := endpointOrDefault("", "https://fallback.example.com/"); got != "https://fallback.example.com/" {
		t.Fatalf("unexpected fallback endpoint result: %q", got)
	}
	if got := joinURL("https://api.example.com/", "/v1/test"); got != "https://api.example.com/v1/test" {
		t.Fatalf("unexpected joinURL result: %q", got)
	}
}
