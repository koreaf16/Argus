package bash

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/koreaf16/argus/internal/services/inventory"
	"github.com/koreaf16/argus/internal/services/workspace"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
)

func TestCheckPermissionDeniesWindowsTargetWithoutBash(t *testing.T) {
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

	input, _ := json.Marshal(map[string]any{
		"command": "ls",
		"server":  "winbox",
	})
	ctx := tool.Context{
		Context:   context.Background(),
		Workspace: mgr,
	}
	got, err := NewBashTool().CheckPermission(ctx, input)
	if err != nil {
		t.Fatalf("check permission: %v", err)
	}
	if got.Behavior != types.BehaviorDeny {
		t.Fatalf("behavior = %s, want %s", got.Behavior, types.BehaviorDeny)
	}
}

func TestCheckPermissionAllowsWindowsTargetWithBash(t *testing.T) {
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
			Shell: "Git Bash /usr/bin/bash",
		},
	})

	input, _ := json.Marshal(map[string]any{
		"command": "ls -la",
		"server":  "winbox",
	})
	ctx := tool.Context{
		Context:   context.Background(),
		Workspace: mgr,
	}
	got, err := NewBashTool().CheckPermission(ctx, input)
	if err != nil {
		t.Fatalf("check permission: %v", err)
	}
	if got.Behavior != types.BehaviorAllow {
		t.Fatalf("behavior = %s, want %s", got.Behavior, types.BehaviorAllow)
	}
}
