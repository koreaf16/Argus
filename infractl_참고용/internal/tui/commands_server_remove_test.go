package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/store"
)

type tuiServerStore struct {
	servers []store.Server
	removed string
}

func (s *tuiServerStore) List(context.Context) ([]store.Server, error) { return s.servers, nil }
func (s *tuiServerStore) Get(context.Context, string) (store.Server, error) {
	return store.Server{}, nil
}
func (s *tuiServerStore) Add(context.Context, store.Server) error    { return nil }
func (s *tuiServerStore) Update(context.Context, store.Server) error { return nil }
func (s *tuiServerStore) Remove(_ context.Context, name string) error {
	s.removed = name
	return nil
}
func (s *tuiServerStore) Close() error { return nil }

type tuiRemoveExec struct {
	target string
	closed bool
}

func (e *tuiRemoveExec) Execute(context.Context, string) (executor.ExecResult, error) {
	return executor.ExecResult{}, nil
}
func (e *tuiRemoveExec) Target() string { return e.target }
func (e *tuiRemoveExec) Host() string   { return e.target }
func (e *tuiRemoveExec) Close() error {
	e.closed = true
	return nil
}

func TestRemoveServerCmdNormalizesNameAndRemovesRuntime(t *testing.T) {
	st := &tuiServerStore{servers: []store.Server{{Name: "sandbox"}}}
	remote := &tuiRemoveExec{target: "sandbox"}
	mgr := executor.NewManager(&tuiRemoveExec{target: "localhost"})
	mgr.Register("sandbox", remote)

	app := AppModel{store: st, manager: mgr}

	msg := app.removeServerCmd(" Sandbox ")()
	if _, ok := msg.(SystemMsg); !ok {
		t.Fatalf("expected SystemMsg, got %#v", msg)
	}
	if st.removed != "sandbox" {
		t.Fatalf("expected canonical store removal, got %q", st.removed)
	}
	if mgr.Has("sandbox") {
		t.Fatalf("expected runtime executor to be removed")
	}
	if !remote.closed {
		t.Fatalf("expected runtime executor to be closed")
	}
}

func TestRemoveServerCmdRejectsMissingServerBeforeRuntimeRemoval(t *testing.T) {
	st := &tuiServerStore{servers: []store.Server{{Name: "sandbox"}}}
	remote := &tuiRemoveExec{target: "ghost"}
	mgr := executor.NewManager(&tuiRemoveExec{target: "localhost"})
	mgr.Register("ghost", remote)

	app := AppModel{store: st, manager: mgr}

	msg := app.removeServerCmd("ghost")()
	errMsg, ok := msg.(ErrorMsg)
	if !ok {
		t.Fatalf("expected ErrorMsg, got %#v", msg)
	}
	if !strings.Contains(errMsg.Err.Error(), "server not found") {
		t.Fatalf("expected server not found error, got %v", errMsg.Err)
	}
	if st.removed != "" {
		t.Fatalf("expected store removal to be skipped")
	}
	if !mgr.Has("ghost") {
		t.Fatalf("expected runtime executor to remain")
	}
	if remote.closed {
		t.Fatalf("expected runtime executor to stay open")
	}
}
