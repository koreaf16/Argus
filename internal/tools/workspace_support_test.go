package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/koreaf16/argus/internal/services/inventory"
	"github.com/koreaf16/argus/internal/services/workspace"
)

func TestResolveValidatedWorkspaceAlias_ExplicitLocalOverridesActive(t *testing.T) {
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
	mgr := workspace.NewManager(reg, nil)
	ctx := Context{Context: context.Background(), Workspace: mgr}

	alias, err := ResolveValidatedWorkspaceAlias(ctx, "local")
	if err != nil {
		t.Fatalf("resolve alias: %v", err)
	}
	if alias != workspace.LocalAlias {
		t.Fatalf("alias = %q, want %q", alias, workspace.LocalAlias)
	}
}

func TestRequiresExplicitServerAlias_MultiRemote(t *testing.T) {
	reg := workspace.NewRegistry("")
	for _, alias := range []string{"oracle-server", "sandbox-server"} {
		if err := reg.Add(workspace.ServerEntry{
			Alias: alias,
			Kind:  workspace.ServerKindSSH,
			Host:  alias + ".example.com",
			Port:  22,
			User:  "oracle",
		}); err != nil {
			t.Fatalf("add server %s: %v", alias, err)
		}
	}
	mgr := workspace.NewManager(reg, nil)
	ctx := Context{Context: context.Background(), Workspace: mgr}

	if !RequiresExplicitServerAlias(ctx) {
		t.Fatal("expected explicit server requirement in multi-remote mode")
	}
	if err := RequireExplicitServerAlias(ctx, "", "bash"); err == nil {
		t.Fatal("expected explicit-server validation error")
	}
	if err := RequireExplicitServerAlias(ctx, "oracle-server", "bash"); err != nil {
		t.Fatalf("unexpected explicit-server validation error: %v", err)
	}
}

func TestRequireExplicitServerAlias_MessageIncludesAliases(t *testing.T) {
	reg := workspace.NewRegistry("")
	for _, alias := range []string{"oracle-server", "sandbox-server"} {
		if err := reg.Add(workspace.ServerEntry{
			Alias: alias,
			Kind:  workspace.ServerKindSSH,
			Host:  alias + ".example.com",
			Port:  22,
			User:  "oracle",
		}); err != nil {
			t.Fatalf("add server %s: %v", alias, err)
		}
	}
	mgr := workspace.NewManager(reg, nil)
	ctx := Context{Context: context.Background(), Workspace: mgr}

	err := RequireExplicitServerAlias(ctx, "", "fileread")
	if err == nil {
		t.Fatal("expected explicit-server validation error")
	}
	msg := err.Error()
	for _, token := range []string{"local", "oracle-server", "sandbox-server"} {
		if !strings.Contains(msg, token) {
			t.Fatalf("error message missing alias %q: %s", token, msg)
		}
	}
}

func TestResolveShellTargetInfo_ReadsSnapshotPlatform(t *testing.T) {
	reg := workspace.NewRegistry("")
	if err := reg.Add(workspace.ServerEntry{
		Alias: "winbox",
		Kind:  workspace.ServerKindSSH,
		Host:  "10.0.0.9",
		Port:  22,
		User:  "admin",
	}); err != nil {
		t.Fatalf("add server: %v", err)
	}
	mgr := workspace.NewManager(reg, nil)
	mgr.SetInventorySnapshot("winbox", inventory.InventorySnapshot{
		Alias:  "winbox",
		Status: inventory.StatusReady,
		System: &inventory.SystemInfo{
			OS:    "Microsoft Windows Server 2022",
			Shell: "PowerShell",
		},
	})

	ctx := Context{Context: context.Background(), Workspace: mgr}
	info, err := ResolveShellTargetInfo(ctx, "winbox", false)
	if err != nil {
		t.Fatalf("resolve shell target: %v", err)
	}
	if info.Platform != workspace.PlatformWindows {
		t.Fatalf("platform = %q, want %q", info.Platform, workspace.PlatformWindows)
	}
	if info.BashAvailable {
		t.Fatalf("expected bash unavailable")
	}
}

func TestValidateShellCompatibility(t *testing.T) {
	err := ValidateShellCompatibility("powershell", ShellTargetInfo{
		Alias:    "linux-box",
		Platform: workspace.PlatformUnix,
	})
	if err == nil {
		t.Fatal("expected powershell compatibility error on unix target")
	}

	err = ValidateShellCompatibility("bash", ShellTargetInfo{
		Alias:         "winbox",
		Platform:      workspace.PlatformWindows,
		BashAvailable: false,
	})
	if err == nil {
		t.Fatal("expected bash compatibility error on windows target without bash")
	}

	if err := ValidateShellCompatibility("bash", ShellTargetInfo{
		Alias:         "winbox",
		Platform:      workspace.PlatformWindows,
		BashAvailable: true,
	}); err != nil {
		t.Fatalf("unexpected bash compatibility error: %v", err)
	}
}
