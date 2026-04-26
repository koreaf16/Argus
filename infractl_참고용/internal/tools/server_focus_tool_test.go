package tools

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/store"
)

type focusTestExec struct {
	target string
	closed bool
}

func (e *focusTestExec) Execute(context.Context, string) (executor.ExecResult, error) {
	return executor.ExecResult{}, nil
}

func (e *focusTestExec) Target() string { return e.target }
func (e *focusTestExec) Host() string   { return e.target }

func (e *focusTestExec) Close() error {
	e.closed = true
	return nil
}

var _ io.Closer = (*focusTestExec)(nil)

type focusTestStore struct {
	servers []store.Server
}

func (s *focusTestStore) List(context.Context) ([]store.Server, error) { return s.servers, nil }
func (s *focusTestStore) Get(context.Context, string) (store.Server, error) {
	return store.Server{}, nil
}
func (s *focusTestStore) Add(context.Context, store.Server) error    { return nil }
func (s *focusTestStore) Update(context.Context, store.Server) error { return nil }
func (s *focusTestStore) Remove(context.Context, string) error       { return nil }
func (s *focusTestStore) Close() error                               { return nil }

func TestServerFocusToolClearDisconnectsActiveServer(t *testing.T) {
	storeStub := &focusTestStore{
		servers: []store.Server{
			{Name: "oracle-db", Host: "192.168.0.120", Port: 22, User: "oracle"},
		},
	}
	local := &focusTestExec{target: "localhost"}
	manager := executor.NewManager(local)
	remote := &focusTestExec{target: "oracle-db"}
	manager.Register("oracle-db", remote)

	var cleared bool
	tool := &ServerFocusTool{
		Store:   storeStub,
		Manager: manager,
		ActiveServer: func() *store.Server {
			return &store.Server{Name: "oracle-db", Host: "192.168.0.120", Port: 22, User: "oracle"}
		},
		OnChange: func(srv *store.Server) {
			cleared = srv == nil
		},
	}

	out, err := tool.Execute(context.Background(), map[string]interface{}{"server": "clear"}, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !out.Success {
		t.Fatalf("expected success, got %+v", out)
	}
	if !cleared {
		t.Fatalf("expected active server to be cleared")
	}
	if manager.Has("oracle-db") {
		t.Fatalf("expected runtime executor to be removed")
	}
	if !remote.closed {
		t.Fatalf("expected remote executor to be closed")
	}
	if !strings.Contains(out.Content, "disconnected") {
		t.Fatalf("expected disconnect message, got %q", out.Content)
	}
}

func TestServerFocusToolKeepsSelectionWhenNotClearing(t *testing.T) {
	storeStub := &focusTestStore{
		servers: []store.Server{
			{Name: "oracle-db", Host: "192.168.0.120", Port: 22, User: "oracle"},
		},
	}
	local := &focusTestExec{target: "localhost"}
	manager := executor.NewManager(local)
	remote := &focusTestExec{target: "oracle-db"}
	manager.Register("oracle-db", remote)

	var selected *store.Server
	tool := &ServerFocusTool{
		Store:   storeStub,
		Manager: manager,
		ActiveServer: func() *store.Server {
			return &store.Server{Name: "oracle-db", Host: "192.168.0.120", Port: 22, User: "oracle"}
		},
		OnChange: func(srv *store.Server) {
			selected = srv
		},
	}

	out, err := tool.Execute(context.Background(), map[string]interface{}{"server": "oracle-db"}, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !out.Success {
		t.Fatalf("expected success, got %+v", out)
	}
	if selected == nil || selected.Name != "oracle-db" {
		t.Fatalf("expected active server selection, got %#v", selected)
	}
	if !manager.Has("oracle-db") {
		t.Fatalf("expected runtime executor to remain connected")
	}
	if remote.closed {
		t.Fatalf("expected remote executor to remain open")
	}
}

func TestServerFocusToolClearWithoutManagerStillClearsFocus(t *testing.T) {
	storeStub := &focusTestStore{
		servers: []store.Server{
			{Name: "oracle-db", Host: "192.168.0.120", Port: 22, User: "oracle"},
		},
	}
	var cleared bool
	tool := &ServerFocusTool{
		Store: storeStub,
		ActiveServer: func() *store.Server {
			return &store.Server{Name: "oracle-db"}
		},
		OnChange: func(srv *store.Server) {
			cleared = srv == nil
		},
	}

	out, err := tool.Execute(context.Background(), map[string]interface{}{"server": "clear"}, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !out.Success {
		t.Fatalf("expected success, got %+v", out)
	}
	if !cleared {
		t.Fatalf("expected active server to be cleared")
	}
}
