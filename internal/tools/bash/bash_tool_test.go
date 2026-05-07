package bash

import (
	"context"
	"encoding/json"
	"strings"
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

func TestResolveBashLocalShell(t *testing.T) {
	cases := []struct {
		name       string
		platform   string
		isRemote   bool
		wantShell  string
	}{
		{name: "windows local falls back to powershell", platform: workspace.PlatformWindows, isRemote: false, wantShell: "powershell"},
		{name: "windows remote keeps bash", platform: workspace.PlatformWindows, isRemote: true, wantShell: "bash"},
		{name: "unix local stays bash", platform: workspace.PlatformUnix, isRemote: false, wantShell: "bash"},
		{name: "unix remote stays bash", platform: workspace.PlatformUnix, isRemote: true, wantShell: "bash"},
		{name: "unknown platform stays bash", platform: "", isRemote: false, wantShell: "bash"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveBashLocalShell(tool.ShellTargetInfo{Platform: tc.platform}, tc.isRemote)
			if got != tc.wantShell {
				t.Fatalf("shell = %q, want %q", got, tc.wantShell)
			}
		})
	}
}

func TestInputSchemaExposesStdinAndBackground(t *testing.T) {
	schema := NewBashTool().InputSchema()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema properties missing")
	}
	stdinSpec, ok := props["stdin"].(map[string]any)
	if !ok {
		t.Fatal("expected stdin field in bash schema")
	}
	if stdinSpec["type"] != "string" {
		t.Fatalf("stdin type = %v, want string", stdinSpec["type"])
	}
	bgSpec, ok := props["background"].(map[string]any)
	if !ok {
		t.Fatal("expected background field in bash schema")
	}
	desc, _ := bgSpec["description"].(string)
	if !strings.Contains(desc, "background_task_id") {
		t.Fatalf("background description should mention background_task_id, got: %s", desc)
	}
}

func TestCallUnmarshalsStdinPayload(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"command": "cat",
		"stdin":   "hello world",
	})
	var req struct {
		Command string `json:"command"`
		Stdin   string `json:"stdin"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.Stdin != "hello world" {
		t.Fatalf("stdin payload mismatch: %q", req.Stdin)
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
