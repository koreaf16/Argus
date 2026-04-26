package web

import (
	"strings"
	"testing"
)

func TestBuildSearchQueryAddsDomainClauses(t *testing.T) {
	got := buildSearchQuery("latest kubernetes docs", SearchOptions{
		AllowedDomains: []string{"https://kubernetes.io/docs", "kubernetes.io"},
		BlockedDomains: []string{"http://example.com/path", "example.com"},
	})

	for _, want := range []string{
		"latest kubernetes docs",
		"site:kubernetes.io",
		"-site:example.com",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q to contain %q", got, want)
		}
	}

	if count := strings.Count(got, "site:kubernetes.io"); count != 1 {
		t.Fatalf("expected site clause once, got %d in %q", count, got)
	}
}

func TestNormalizeSearchDomain(t *testing.T) {
	for raw, want := range map[string]string{
		"https://docs.djangoproject.com/en/5.0/": "docs.djangoproject.com",
		"http://www.gnu.org/software/bash/":      "gnu.org",
		"site:example.com/path":                  "example.com",
	} {
		if got := normalizeSearchDomain(raw); got != want {
			t.Fatalf("normalizeSearchDomain(%q) = %q, want %q", raw, got, want)
		}
	}
}
