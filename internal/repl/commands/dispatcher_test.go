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
