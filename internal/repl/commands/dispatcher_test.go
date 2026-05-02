package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/koreaf16/argus/internal/services/workspace"
	"github.com/koreaf16/argus/internal/state"
)

func TestDispatchUnknownCommand(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	_, err := Dispatch("/does-not-exist", CommandContext{
		Context: context.Background(),
		Stdout:  &out,
		State:   state.NewAppState(),
	})
	if err == nil {
		t.Fatalf("expected error for unknown command")
	}
}

func TestDispatchElevateCommandRemoved(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	_, err := Dispatch("/elevate a100-server", CommandContext{
		Context: context.Background(),
		Stdout:  &out,
		State:   state.NewAppState(),
	})
	if err == nil {
		t.Fatalf("expected error for removed /elevate command")
	}
	if !strings.Contains(err.Error(), "unknown command: /elevate") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDispatchHelpDoesNotListElevate(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	_, err := Dispatch("/help", CommandContext{
		Context: context.Background(),
		Stdout:  &out,
		State:   state.NewAppState(),
	})
	if err != nil {
		t.Fatalf("help failed: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "/elevate") {
		t.Fatalf("help still lists /elevate: %s", got)
	}
	if !strings.Contains(got, "/server list|add|edit|connect") {
		t.Fatalf("help does not advertise /server edit: %s", got)
	}
}

func TestDispatchCommitNonGitRepo(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	_, err := Dispatch("/commit test", CommandContext{
		Context: context.Background(),
		Stdout:  &out,
		WorkDir: t.TempDir(),
		State:   state.NewAppState(),
	})
	if err == nil {
		t.Fatalf("expected error for non-git repository")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDispatchConfigSetAndGet(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"ui":{"theme":"default"}}`), 0o600); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	var out strings.Builder
	ctx := CommandContext{
		Context:      context.Background(),
		Stdout:       &out,
		SettingsPath: settingsPath,
		State:        state.NewAppState(),
	}
	if _, err := Dispatch(`/config set ui.theme "solarized"`, ctx); err != nil {
		t.Fatalf("set config: %v", err)
	}
	if _, err := Dispatch(`/config get ui.theme`, ctx); err != nil {
		t.Fatalf("get config: %v", err)
	}
	if !strings.Contains(out.String(), `"solarized"`) {
		t.Fatalf("expected updated value, got: %s", out.String())
	}
}

func TestServerEditCommandInvokesFormAndSaves(t *testing.T) {
	t.Parallel()
	registryPath := filepath.Join(t.TempDir(), "servers.json")
	reg := workspace.NewRegistry(registryPath)
	if err := reg.Add(workspace.ServerEntry{
		Alias: "a100-server",
		Kind:  workspace.ServerKindSSH,
		Host:  "10.0.0.8",
		Port:  22,
		User:  "ubuntu",
	}); err != nil {
		t.Fatalf("add server: %v", err)
	}
	ws := workspace.NewManager(reg, nil)

	var out strings.Builder
	var requestedAlias string
	_, err := Dispatch("/server edit a100-server", CommandContext{
		Context:   context.Background(),
		Stdout:    &out,
		Workspace: ws,
		State:     state.NewAppState(),
		ServerFormPrompt: func(_ context.Context, req ServerFormRequest) (ServerFormResult, error) {
			requestedAlias = req.EditAlias
			return ServerFormResult{Entry: workspace.ServerEntry{
				Alias: "a100-server",
				Kind:  workspace.ServerKindSSH,
				Host:  "10.0.0.9",
				Port:  2222,
				User:  "admin",
				Elevation: workspace.Elevation{
					Allowed:     true,
					Mode:        "reuse_login",
					TargetUsers: []string{"root"},
				},
			}}, nil
		},
	})
	if err != nil {
		t.Fatalf("dispatch server edit: %v", err)
	}
	if requestedAlias != "a100-server" {
		t.Fatalf("prompt edit alias = %q, want a100-server", requestedAlias)
	}
	entry, ok := reg.Get("a100-server")
	if !ok {
		t.Fatal("edited server missing from registry")
	}
	if entry.Host != "10.0.0.9" || entry.Port != 2222 || entry.User != "admin" {
		t.Fatalf("server was not updated: %+v", entry)
	}
	if !entry.Elevation.Allowed || entry.Elevation.Mode != "reuse_login" || len(entry.Elevation.TargetUsers) != 1 || entry.Elevation.TargetUsers[0] != "root" {
		t.Fatalf("elevation was not updated: %+v", entry.Elevation)
	}
	if !strings.Contains(out.String(), "updated server: a100-server") {
		t.Fatalf("missing update summary: %s", out.String())
	}
}

func TestServerDisconnectWithoutAliasResetsActiveWorkspace(t *testing.T) {
	reg := workspace.NewRegistry("")
	if err := reg.Add(workspace.ServerEntry{
		Alias: "a100-server",
		Kind:  workspace.ServerKindSSH,
		Host:  "10.0.0.8",
		Port:  22,
		User:  "ubuntu",
	}); err != nil {
		t.Fatalf("add server: %v", err)
	}
	if err := reg.SetActive("a100-server"); err != nil {
		t.Fatalf("set active: %v", err)
	}
	ws := workspace.NewManager(reg, nil)
	appState := state.NewAppState()
	appState.SetActiveWorkspace("a100-server")

	var out strings.Builder
	_, err := Dispatch("/server disconnect", CommandContext{
		Context:   context.Background(),
		Stdout:    &out,
		Workspace: ws,
		State:     appState,
	})
	if err != nil {
		t.Fatalf("dispatch disconnect: %v", err)
	}
	if ws.ActiveAlias() != workspace.LocalAlias {
		t.Fatalf("active alias = %q, want %q", ws.ActiveAlias(), workspace.LocalAlias)
	}
	if appState.ActiveWorkspace() != workspace.LocalAlias {
		t.Fatalf("state active workspace = %q, want %q", appState.ActiveWorkspace(), workspace.LocalAlias)
	}
}

func TestServerAccountAddCreatesWorkTarget(t *testing.T) {
	registryPath := filepath.Join(t.TempDir(), "servers.json")
	reg := workspace.NewRegistry(registryPath)
	if err := reg.Add(workspace.ServerEntry{
		Alias: "parent",
		Kind:  workspace.ServerKindSSH,
		Host:  "10.0.0.8",
		Port:  22,
		User:  "master",
	}); err != nil {
		t.Fatalf("add parent: %v", err)
	}
	ws := workspace.NewManager(reg, nil)

	var out strings.Builder
	_, err := Dispatch("/server account add parent app --alias parent-app --method su --cwd /srv/app", CommandContext{
		Context:   context.Background(),
		Stdout:    &out,
		Workspace: ws,
		State:     state.NewAppState(),
	})
	if err != nil {
		t.Fatalf("dispatch account add: %v", err)
	}
	entry, ok := reg.Get("parent-app")
	if !ok {
		t.Fatal("account target missing")
	}
	if entry.Kind != workspace.ServerKindAccount || entry.ParentAlias != "parent" || entry.User != "app" || entry.SwitchMethod != workspace.PrivilegeSU {
		t.Fatalf("unexpected account target: %+v", entry)
	}
	if entry.DefaultCWD != "/srv/app" {
		t.Fatalf("default cwd = %q", entry.DefaultCWD)
	}
	if !strings.Contains(out.String(), "added work account: parent-app") {
		t.Fatalf("missing account add summary: %s", out.String())
	}
}
