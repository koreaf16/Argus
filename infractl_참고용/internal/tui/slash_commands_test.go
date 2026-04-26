package tui

import "testing"

func TestResolveSlashCommandContext(t *testing.T) {
	def, ok := resolveSlashCommand("/context")
	if !ok {
		t.Fatal("expected /context to resolve")
	}
	if def.Name != "/context" {
		t.Fatalf("expected /context definition, got %q", def.Name)
	}
}

func TestSlashCommandMatchesPrefixIncludesContext(t *testing.T) {
	matches := slashCommandMatchesPrefix("/cont")
	for _, def := range matches {
		if def.Name == "/context" {
			return
		}
	}
	t.Fatalf("expected /context in matches, got %+v", matches)
}

func TestResolveSlashCommandWorkspaceAliases(t *testing.T) {
	def, ok := resolveSlashCommand("/server")
	if !ok {
		t.Fatal("expected /server alias to resolve")
	}
	if def.Name != "/workspace" {
		t.Fatalf("expected /workspace definition, got %q", def.Name)
	}

	def, ok = resolveSlashCommand("/servers")
	if !ok {
		t.Fatal("expected /servers alias to resolve")
	}
	if def.Name != "/workspaces" {
		t.Fatalf("expected /workspaces definition, got %q", def.Name)
	}
}
