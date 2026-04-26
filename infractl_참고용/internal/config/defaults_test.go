package config

import "testing"

func TestApplyDefaultsSetsSearchDefaultsWhenMissing(t *testing.T) {
	cfg := &Config{}

	applyDefaults(cfg)

	if cfg.Search.TimeoutSeconds != 10 {
		t.Fatalf("expected timeout_seconds=10, got %d", cfg.Search.TimeoutSeconds)
	}
	if cfg.Search.CacheTTLSeconds != 120 {
		t.Fatalf("expected cache_ttl_seconds=120, got %d", cfg.Search.CacheTTLSeconds)
	}
	assertBoolPtr(t, "github", cfg.Search.Adapters.GitHub, true)
	assertBoolPtr(t, "huggingface", cfg.Search.Adapters.HuggingFace, true)
	assertBoolPtr(t, "dockerhub", cfg.Search.Adapters.DockerHub, true)
	assertBoolPtr(t, "pypi", cfg.Search.Adapters.PyPI, true)
	assertBoolPtr(t, "npm", cfg.Search.Adapters.NPM, true)
	assertBoolPtr(t, "osv", cfg.Search.Adapters.OSV, true)
}

func TestApplyDefaultsPreservesExplicitSearchAdapterValues(t *testing.T) {
	f := false
	cfg := &Config{}
	cfg.Search.TimeoutSeconds = 30
	cfg.Search.CacheTTLSeconds = 15
	cfg.Search.Adapters.HuggingFace = &f

	applyDefaults(cfg)

	if cfg.Search.TimeoutSeconds != 30 {
		t.Fatalf("expected timeout_seconds to remain 30, got %d", cfg.Search.TimeoutSeconds)
	}
	if cfg.Search.CacheTTLSeconds != 15 {
		t.Fatalf("expected cache_ttl_seconds to remain 15, got %d", cfg.Search.CacheTTLSeconds)
	}
	assertBoolPtr(t, "huggingface", cfg.Search.Adapters.HuggingFace, false)
	assertBoolPtr(t, "github", cfg.Search.Adapters.GitHub, true)
	assertBoolPtr(t, "dockerhub", cfg.Search.Adapters.DockerHub, true)
	assertBoolPtr(t, "pypi", cfg.Search.Adapters.PyPI, true)
	assertBoolPtr(t, "npm", cfg.Search.Adapters.NPM, true)
	assertBoolPtr(t, "osv", cfg.Search.Adapters.OSV, true)
}

func assertBoolPtr(t *testing.T, name string, got *bool, want bool) {
	t.Helper()
	if got == nil {
		t.Fatalf("expected %s pointer to be set", name)
	}
	if *got != want {
		t.Fatalf("expected %s=%v, got %v", name, want, *got)
	}
}
