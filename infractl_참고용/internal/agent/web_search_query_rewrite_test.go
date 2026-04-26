package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/store"
)

type rewriteServerStore struct {
	servers map[string]store.Server
}

func (s rewriteServerStore) List(context.Context) ([]store.Server, error) {
	out := make([]store.Server, 0, len(s.servers))
	for _, srv := range s.servers {
		out = append(out, srv)
	}
	return out, nil
}

func (s rewriteServerStore) Get(_ context.Context, name string) (store.Server, error) {
	if srv, ok := s.servers[name]; ok {
		return srv, nil
	}
	return store.Server{}, context.Canceled
}

func (s rewriteServerStore) Add(context.Context, store.Server) error    { return nil }
func (s rewriteServerStore) Update(context.Context, store.Server) error { return nil }
func (s rewriteServerStore) Remove(context.Context, string) error       { return nil }
func (s rewriteServerStore) Close() error                               { return nil }

func TestNormalizeWebSearchQueryByOSAddsRockyAndRHELHints(t *testing.T) {
	args := map[string]interface{}{
		"query": "Oracle Database 19c installation guide",
	}
	active := &store.Server{Name: "sandbox", OS: "Rocky Linux 9.7 (Blue Onyx)"}

	normalizeWebSearchQueryByOS(context.Background(), args, "sandbox", active, nil)

	got, _ := args["query"].(string)
	if got == "" {
		t.Fatalf("expected rewritten query, got empty")
	}
	for _, want := range []string{"Rocky Linux 9", "RHEL 9 compatible"} {
		if !containsFold(got, want) {
			t.Fatalf("expected %q in query, got %q", want, got)
		}
	}
}

func TestNormalizeWebSearchQueryByOSAvoidsDuplicateHints(t *testing.T) {
	query := "Oracle 19c install Rocky Linux 9 RHEL 9 compatible"
	args := map[string]interface{}{"query": query}
	active := &store.Server{Name: "sandbox", OS: "Rocky Linux 9.7"}

	normalizeWebSearchQueryByOS(context.Background(), args, "sandbox", active, nil)

	got, _ := args["query"].(string)
	if got != query {
		t.Fatalf("expected unchanged query, got %q", got)
	}
}

func TestNormalizeWebSearchQueryByOSSkipsGeneralQuery(t *testing.T) {
	query := "Oracle licensing model overview"
	args := map[string]interface{}{"query": query}
	active := &store.Server{Name: "sandbox", OS: "Rocky Linux 9.7"}

	normalizeWebSearchQueryByOS(context.Background(), args, "sandbox", active, nil)

	got, _ := args["query"].(string)
	if got != query {
		t.Fatalf("expected unchanged query for non-install/search intent, got %q", got)
	}
}

func TestNormalizeWebSearchQueryByOSUsesTargetServerOSFromStore(t *testing.T) {
	query := "Oracle 19c setup guide"
	args := map[string]interface{}{"query": query}
	active := &store.Server{Name: "other", OS: "Ubuntu 22.04"}
	srvStore := rewriteServerStore{
		servers: map[string]store.Server{
			"sandbox": {Name: "sandbox", OS: "Rocky Linux 9.7"},
		},
	}

	normalizeWebSearchQueryByOS(context.Background(), args, "sandbox", active, srvStore)

	got, _ := args["query"].(string)
	for _, want := range []string{"Rocky Linux 9", "RHEL 9 compatible"} {
		if !containsFold(got, want) {
			t.Fatalf("expected %q in query, got %q", want, got)
		}
	}
}

func TestNormalizeWebSearchQueryByOSNoVersionLeavesQueryUnchanged(t *testing.T) {
	query := "Oracle 19c installation guide"
	args := map[string]interface{}{"query": query}
	active := &store.Server{Name: "sandbox", OS: "Rocky Linux"}

	normalizeWebSearchQueryByOS(context.Background(), args, "sandbox", active, nil)

	got, _ := args["query"].(string)
	if got != query {
		t.Fatalf("expected unchanged query when version is missing, got %q", got)
	}
}

func TestNormalizeWebSearchQueryByOSRewritesQueriesArray(t *testing.T) {
	args := map[string]interface{}{
		"queries": []interface{}{
			"oracle 19c installation guide",
			"oracle release notes",
		},
	}
	active := &store.Server{Name: "sandbox", OS: "Rocky Linux 9.7"}

	normalizeWebSearchQueryByOS(context.Background(), args, "sandbox", active, nil)

	got, ok := args["queries"].([]interface{})
	if !ok {
		t.Fatalf("expected []interface{} queries after rewrite, got %T", args["queries"])
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 queries, got %d", len(got))
	}
	first, _ := got[0].(string)
	second, _ := got[1].(string)
	if !containsFold(first, "Rocky Linux 9") || !containsFold(first, "RHEL 9 compatible") {
		t.Fatalf("expected first query to include OS hints, got %q", first)
	}
	if second != "oracle release notes" {
		t.Fatalf("expected non-environment-sensitive query unchanged, got %q", second)
	}
}

func TestNormalizeWebSearchQueryByOSRewritesStringSliceQueriesArray(t *testing.T) {
	args := map[string]interface{}{
		"queries": []string{
			"oracle setup guide",
			"oracle feature summary",
		},
	}
	active := &store.Server{Name: "sandbox", OS: "Ubuntu 24.04"}

	normalizeWebSearchQueryByOS(context.Background(), args, "sandbox", active, nil)

	got, ok := args["queries"].([]string)
	if !ok {
		t.Fatalf("expected []string queries after rewrite, got %T", args["queries"])
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 queries, got %d", len(got))
	}
	if !containsFold(got[0], "Ubuntu 24") {
		t.Fatalf("expected first query to include Ubuntu hint, got %q", got[0])
	}
	if got[1] != "oracle feature summary" {
		t.Fatalf("expected non-sensitive query unchanged, got %q", got[1])
	}
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
