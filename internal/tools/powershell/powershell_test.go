package powershell

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

func TestInputSchemaIncludesServer(t *testing.T) {
	schema := NewPowerShellTool().InputSchema()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties missing")
	}
	if _, ok := props["server"]; !ok {
		t.Fatalf("expected server property in powershell input schema")
	}
}

func TestInputSchemaExposesStdinAndBackground(t *testing.T) {
	schema := NewPowerShellTool().InputSchema()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema properties missing")
	}
	stdinSpec, ok := props["stdin"].(map[string]any)
	if !ok {
		t.Fatal("expected stdin field in powershell schema")
	}
	if stdinSpec["type"] != "string" {
		t.Fatalf("stdin type = %v, want string", stdinSpec["type"])
	}
	bgSpec, ok := props["background"].(map[string]any)
	if !ok {
		t.Fatal("expected background field in powershell schema")
	}
	desc, _ := bgSpec["description"].(string)
	if !strings.Contains(desc, "background_task_id") {
		t.Fatalf("background description should mention background_task_id, got: %s", desc)
	}
}

func TestCheckPermissionDeniesUnixTarget(t *testing.T) {
	reg := workspace.NewRegistry("")
	if err := reg.Add(workspace.ServerEntry{
		Alias: "linux-box",
		Kind:  workspace.ServerKindSSH,
		Host:  "10.0.0.8",
		Port:  22,
		User:  "ubuntu",
	}); err != nil {
		t.Fatalf("add server: %v", err)
	}
	mgr := workspace.NewManager(reg, nil)
	mgr.SetInventorySnapshot("linux-box", inventory.InventorySnapshot{
		Alias:  "linux-box",
		Status: inventory.StatusReady,
		System: &inventory.SystemInfo{
			OS:    "Linux 6.8.0",
			Shell: "/bin/bash",
		},
	})

	input, _ := json.Marshal(map[string]any{
		"command": "Get-Process",
		"server":  "linux-box",
	})
	ctx := tool.Context{
		Context:   context.Background(),
		Workspace: mgr,
	}
	got, err := NewPowerShellTool().CheckPermission(ctx, input)
	if err != nil {
		t.Fatalf("check permission: %v", err)
	}
	if got.Behavior != types.BehaviorDeny {
		t.Fatalf("behavior = %s, want %s", got.Behavior, types.BehaviorDeny)
	}
}
