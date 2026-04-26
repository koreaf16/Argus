package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/store"
)

type serverAddStore struct {
	servers []store.Server
}

func (s *serverAddStore) List(context.Context) ([]store.Server, error) { return s.servers, nil }
func (s *serverAddStore) Get(context.Context, string) (store.Server, error) {
	return store.Server{}, nil
}
func (s *serverAddStore) Add(context.Context, store.Server) error    { return nil }
func (s *serverAddStore) Update(context.Context, store.Server) error { return nil }
func (s *serverAddStore) Remove(context.Context, string) error       { return nil }
func (s *serverAddStore) Close() error                               { return nil }

func TestServerAddToolRejectsDuplicateBeforeSSH(t *testing.T) {
	tool := &ServerAddTool{
		Store: &serverAddStore{servers: []store.Server{
			{Name: "sandbox", Host: "192.168.0.130", Port: 22, User: "sandbox"},
		}},
	}

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"name":       " Sandbox ",
		"host":       "192.168.0.130",
		"user":       "sandbox",
		"auth_type":  "password",
		"credential": "secret",
	}, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if out.Success {
		t.Fatalf("expected duplicate add to fail, got %+v", out)
	}
	if !strings.Contains(out.Content, "already registered") {
		t.Fatalf("expected duplicate diagnostic, got %q", out.Content)
	}
}

func TestServerAddToolRejectsInvalidAuthType(t *testing.T) {
	tool := &ServerAddTool{Store: &serverAddStore{}}

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"name":       "newbox",
		"host":       "192.168.0.130",
		"user":       "sandbox",
		"auth_type":  "ssh",
		"credential": "secret",
	}, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if out.Success {
		t.Fatalf("expected invalid auth_type to fail, got %+v", out)
	}
	if !strings.Contains(out.Content, "auth_type") {
		t.Fatalf("expected auth_type diagnostic, got %q", out.Content)
	}
}

func TestServerAddToolRejectsMissingTrimmedFields(t *testing.T) {
	tool := &ServerAddTool{Store: &serverAddStore{}}

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"name":       "   ",
		"host":       "192.168.0.130",
		"user":       "sandbox",
		"auth_type":  "password",
		"credential": "secret",
	}, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if out.Success {
		t.Fatalf("expected blank name to fail, got %+v", out)
	}
	if !strings.Contains(out.Content, "name") {
		t.Fatalf("expected name diagnostic, got %q", out.Content)
	}
}

func TestServerAddToolRejectsReservedLocalAlias(t *testing.T) {
	tool := &ServerAddTool{Store: &serverAddStore{}}

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"name":       "localhost",
		"host":       "192.168.0.130",
		"user":       "sandbox",
		"auth_type":  "password",
		"credential": "secret",
	}, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if out.Success {
		t.Fatalf("expected reserved local alias to fail, got %+v", out)
	}
	if !strings.Contains(out.Content, "reserved") {
		t.Fatalf("expected reserved alias diagnostic, got %q", out.Content)
	}
}

func TestServerAddToolRejectsWhitespaceAlias(t *testing.T) {
	tool := &ServerAddTool{Store: &serverAddStore{}}

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"name":       "prod db",
		"host":       "192.168.0.130",
		"user":       "sandbox",
		"auth_type":  "password",
		"credential": "secret",
	}, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if out.Success {
		t.Fatalf("expected whitespace alias to fail, got %+v", out)
	}
	if !strings.Contains(out.Content, "invalid") {
		t.Fatalf("expected invalid alias diagnostic, got %q", out.Content)
	}
}

func TestServerAddToolRejectsMissingManagerBeforeSSH(t *testing.T) {
	tool := &ServerAddTool{Store: &serverAddStore{}}

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"name":       "newbox",
		"host":       "192.0.2.10",
		"user":       "sandbox",
		"auth_type":  "password",
		"credential": "secret",
	}, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if out.Success {
		t.Fatalf("expected missing manager to fail, got %+v", out)
	}
	if !strings.Contains(out.Content, "executor manager") {
		t.Fatalf("expected manager diagnostic, got %q", out.Content)
	}
}
